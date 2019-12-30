all:
	protoc -I/usr/local/include -I. \
		-I/home/bwilson/Downloads/include \
		-I${GOPATH}/src/github.com/googleapis/googleapis \
		--go_out=plugins=grpc:. \
		service.proto
	protoc -I/usr/local/include -I. \
		-I/home/bwilson/Downloads/include \
		-I${GOPATH}/src/github.com/googleapis/googleapis \
		--grpc-gateway_out=logtostderr=true:. \
		service.proto