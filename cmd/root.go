package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/zcash-hackworks/lightwalletd/common"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lightwalletd",
	Short: "Lightwalletd is a backend service to the Zcash blockchain",
	Long: `Lightwalletd is a backend service that provides a 
         bandwidth-efficient interface to the Zcash blockchain`,
	Run: func(cmd *cobra.Command, args []string) {
		opts := &common.Options{
			BindAddr:      viper.GetString("bind-addr"),
			TLSCertPath:   viper.GetString("tls-cert"),
			TLSKeyPath:    viper.GetString("tls-key"),
			LogLevel:      viper.GetInt("log-level"),
			LogFile:       viper.GetString("log-file"),
			ZcashConfPath: viper.GetString("zcash-conf-path"),
			VeryInsecure:  viper.GetBool("very-insecure"),
			CacheSize:     viper.GetInt("cache-size"),
		}

		fmt.Printf("Options: %#v", opts)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is current directory, lightwalletd.yaml)")
	rootCmd.Flags().String("bind-addr", "127.0.0.1:9067", "the address to listen on")
	rootCmd.Flags().String("tls-cert", "./cert.pem", "the path to a TLS certificate")
	rootCmd.Flags().String("tls-key", "./cert.key", "the path to a TLS key file")
	rootCmd.Flags().Int("log-level", int(logrus.InfoLevel), "log level (logrus 1-7)")
	rootCmd.Flags().String("log-file", "./server.log", "log file to write to")
	rootCmd.Flags().String("zcash-conf-path", "./zcash.conf", "conf file to pull RPC creds from")
	rootCmd.Flags().Bool("no-tls-very-insecure", false, "run without the required TLS certificate, only for debugging, DO NOT use in production")
	rootCmd.Flags().Int("cache-size", 80000, "number of blocks to hold in the cache")

	viper.BindPFlag("bind-addr", rootCmd.Flags().Lookup("bind-addr"))
	viper.SetDefault("bind-addr", "127.0.0.1:9067")
	viper.BindPFlag("tls-cert", rootCmd.Flags().Lookup("tls-cert"))
	viper.SetDefault("tls-cert", "./cert.pem")
	viper.BindPFlag("tls-key", rootCmd.Flags().Lookup("tls-key"))
	viper.SetDefault("tls-key", "./cert.key")
	viper.BindPFlag("log-level", rootCmd.Flags().Lookup("log-level"))
	viper.SetDefault("log-level", int(logrus.InfoLevel))
	viper.BindPFlag("log-file", rootCmd.Flags().Lookup("log-file"))
	viper.SetDefault("log-file", "./server.log")
	viper.BindPFlag("zcash-conf-path", rootCmd.Flags().Lookup("zcash-conf-path"))
	viper.SetDefault("zcash-conf-path", "./zcash.conf")
	viper.BindPFlag("no-tls-very-insecure", rootCmd.Flags().Lookup("no-tls-very-insecure"))
	viper.SetDefault("no-tls-very-insecure", false)
	viper.BindPFlag("cache-size", rootCmd.Flags().Lookup("cache-size"))
	viper.SetDefault("cache-size", 80000)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("lightwalletd")
	}

	// Replace `-` in config options with `_` for ENV keys
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv() // read in environment variables that match
	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

}
