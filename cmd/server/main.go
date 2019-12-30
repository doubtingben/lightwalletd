package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"

	"github.com/zcash-hackworks/lightwalletd/common"
	"github.com/zcash-hackworks/lightwalletd/frontend"
	"github.com/zcash-hackworks/lightwalletd/walletrpc"
)

var log *logrus.Entry
var logger = logrus.New()

func init() {
	logger.SetFormatter(&logrus.TextFormatter{
		//DisableColors:          true,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
	})

	onexit := func() {
		fmt.Printf("Lightwalletd died with a Fatal error. Check logfile for details.\n")
	}

	log = logger.WithFields(logrus.Fields{
		"app": "frontend-grpc",
	})

	logrus.RegisterExitHandler(onexit)
}

// TODO stream logging

func LoggingInterceptor() grpc.ServerOption {
	return grpc.UnaryInterceptor(logInterceptor)
}

func logInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	reqLog := loggerFromContext(ctx)
	start := time.Now()

	resp, err := handler(ctx, req)

	entry := reqLog.WithFields(logrus.Fields{
		"method":   info.FullMethod,
		"duration": time.Since(start),
		"error":    err,
	})

	if err != nil {
		entry.Error("call failed")
	} else {
		entry.Info("method called")
	}

	return resp, err
}

func loggerFromContext(ctx context.Context) *logrus.Entry {
	// TODO: anonymize the addresses. cryptopan?
	if peerInfo, ok := peer.FromContext(ctx); ok {
		return log.WithFields(logrus.Fields{"peer_addr": peerInfo.Addr})
	}
	return log.WithFields(logrus.Fields{"peer_addr": "unknown"})
}

type Options struct {
	bindAddr      string `json:"bind_address,omitempty"`
	tlsCertPath   string `json:"tls_cert_path,omitempty"`
	tlsKeyPath    string `json:"tls_cert_key,omitempty"`
	logLevel      uint64 `json:"log_level,omitempty"`
	logPath       string `json:"log_file,omitempty"`
	zcashConfPath string `json:"zcash_conf,omitempty"`
	veryInsecure  bool   `json:"very_insecure,omitempty"`
	cacheSize     int    `json:"cache_size,omitempty"`
	wantVersion   bool
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// grpcHandlerFunc returns an http.Handler that delegates to grpcServer on incoming gRPC
// connections or otherHandler otherwise. Copied from cockroachdb.
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO(tamird): point to merged gRPC code rather than a PR.
		// This is a partial recreation of gRPC's internal checks https://github.com/grpc/grpc-go/pull/514/files#diff-95e9a25b738459a2d3030e1e6fa2a718R61
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	})
}

func main() {
	opts := &Options{}
	flag.StringVar(&opts.bindAddr, "bind-addr", "127.0.0.1:9067", "the address to listen on")
	flag.StringVar(&opts.tlsCertPath, "tls-cert", "./cert.pem", "the path to a TLS certificate")
	flag.StringVar(&opts.tlsKeyPath, "tls-key", "./cert.key", "the path to a TLS key file")
	flag.Uint64Var(&opts.logLevel, "log-level", uint64(logrus.InfoLevel), "log level (logrus 1-7)")
	flag.StringVar(&opts.logPath, "log-file", "./server.log", "log file to write to")
	flag.StringVar(&opts.zcashConfPath, "conf-file", "./zcash.conf", "conf file to pull RPC creds from")
	flag.BoolVar(&opts.veryInsecure, "no-tls-very-insecure", false, "run without the required TLS certificate, only for debugging, DO NOT use in production")
	flag.BoolVar(&opts.wantVersion, "version", false, "version (major.minor.patch)")
	flag.IntVar(&opts.cacheSize, "cache-size", 80000, "number of blocks to hold in the cache")

	// TODO prod metrics
	// TODO support config from file and env vars
	flag.Parse()

	if opts.wantVersion {
		fmt.Println("lightwalletd version v0.2.0")
		return
	}

	filesThatShouldExist := []string{
		opts.tlsCertPath,
		opts.tlsKeyPath,
		opts.logPath,
		opts.zcashConfPath,
	}

	for _, filename := range filesThatShouldExist {
		if !fileExists(opts.logPath) {
			os.OpenFile(opts.logPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
		}
		if opts.veryInsecure && (filename == opts.tlsCertPath || filename == opts.tlsKeyPath) {
			continue
		}
		if !fileExists(filename) {
			os.Stderr.WriteString(fmt.Sprintf("\n  ** File does not exist: %s\n\n", filename))
			flag.Usage()
			os.Exit(1)
		}
	}

	if opts.logPath != "" {
		// instead write parsable logs for logstash/splunk/etc
		output, err := os.OpenFile(opts.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.WithFields(logrus.Fields{
				"error": err,
				"path":  opts.logPath,
			}).Fatal("couldn't open log file")
		}
		defer output.Close()
		logger.SetOutput(output)
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

	logger.SetLevel(logrus.Level(opts.logLevel))

	// gRPC initialization
	var server *grpc.Server

	if opts.veryInsecure {
		server = grpc.NewServer(LoggingInterceptor())
	} else {
		transportCreds, err := credentials.NewServerTLSFromFile(opts.tlsCertPath, opts.tlsKeyPath)
		if err != nil {
			log.WithFields(logrus.Fields{
				"cert_file": opts.tlsCertPath,
				"key_path":  opts.tlsKeyPath,
				"error":     err,
			}).Fatal("couldn't load TLS credentials")
		}
		server = grpc.NewServer(grpc.Creds(transportCreds), LoggingInterceptor())
	}

	// Enable reflection for debugging
	if opts.logLevel >= uint64(logrus.WarnLevel) {
		reflection.Register(server)
	}

	// Initialize Zcash RPC client. Right now (Jan 2018) this is only for
	// sending transactions, but in the future it could back a different type
	// of block streamer.

	rpcClient, err := frontend.NewZRPCFromConf(opts.zcashConfPath)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("setting up RPC connection to zcashd")
	}

	// Get the sapling activation height from the RPC
	// (this first RPC also verifies that we can communicate with zcashd)
	saplingHeight, blockHeight, chainName, branchID := common.GetSaplingInfo(rpcClient, log)
	log.Info("Got sapling height ", saplingHeight, " chain ", chainName, " branchID ", branchID)

	// Initialize the cache
	cache := common.NewBlockCache(opts.cacheSize)

	// Start the block cache importer at cacheSize blocks before current height
	cacheStart := blockHeight - opts.cacheSize
	if cacheStart < saplingHeight {
		cacheStart = saplingHeight
	}

	go common.BlockIngestor(rpcClient, cache, log, cacheStart)

	// Compact transaction service initialization
	service, err := frontend.NewLwdStreamer(rpcClient, cache, log)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("couldn't create backend")
	}

	mux := http.NewServeMux()

	// Register service
	walletrpc.RegisterCompactTxStreamerServer(server, service)
	ctx := context.Background()
	gwmux := runtime.NewServeMux()
	walletrpc.RegisterCompactTxStreamerHandlerFromEndpoint(ctx, gwmux, opts.bindAddr, []grpc.DialOption{})

	// Start listening
	listener, err := net.Listen("tcp", opts.bindAddr)
	if err != nil {
		log.WithFields(logrus.Fields{
			"bind_addr": opts.bindAddr,
			"error":     err,
		}).Fatal("couldn't create listener")
	}

	srv := &http.Server{
		Addr:    opts.bindAddr,
		Handler: grpcHandlerFunc(server, mux),
		TLSConfig: &tls.Config{
			NextProtos: []string{"h2"},
		},
	}

	err = srv.Serve(tls.NewListener(listener, srv.TLSConfig))

	// Signal handler for graceful stops
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-signals
		log.WithFields(logrus.Fields{
			"signal": s.String(),
		}).Info("caught signal, stopping gRPC server")
		os.Exit(1)
	}()

	log.Infof("Starting gRPC server on %s", opts.bindAddr)
	err = server.Serve(listener)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("gRPC server exited")
	}
}
