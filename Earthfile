VERSION 0.8

FROM golang:1.26.0-alpine

# install gcc dependencies into alpine for CGO
RUN apk --no-cache add git ca-certificates gcc musl-dev libc-dev binutils-gold curl openssh

# install docker tools
# https://docs.docker.com/engine/install/debian/
RUN apk add --update --no-cache docker

# install the go generator plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
RUN export PATH="$PATH:$(go env GOPATH)/bin"

# install buf from source
RUN GO111MODULE=on GOBIN=/usr/local/bin go install github.com/bufbuild/buf/cmd/buf@v1.59.0

# install oapi to generate swagger
RUN go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0

# install the various tools to generate connect-go
RUN go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
RUN go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

all:
	BUILD +protogen
	BUILD +opengen
	BUILD +opengen-k8s-v2
	BUILD +opengen-saga

code:
    WORKDIR /app

    # download deps
    COPY go.mod go.sum ./
    RUN go mod download -x

    # copy in code
    COPY --dir . ./

vendor:
    FROM +code

    RUN go mod tidy && go mod vendor
    SAVE ARTIFACT /app /files


protogen:
    # copy the proto files to generate
    COPY --dir protos/ ./
    COPY buf.yaml buf.gen.yaml ./

    # generate the pbs
    RUN buf generate \
            --template buf.gen.yaml \
            --path protos/sample \
            --path protos/helloworld \
            --path protos/chat

    # save artifact to
    SAVE ARTIFACT gen/sample AS LOCAL internal/samplepb
    SAVE ARTIFACT gen/chat AS LOCAL internal/chatpb
    SAVE ARTIFACT gen/helloworld AS LOCAL internal/helloworldpb

opengen:
    WORKDIR /app

    COPY goakt-actors-cluster/dnssd-v2/api/openapi.yaml goakt-actors-cluster/dnssd-v2/api/cfg.yaml goakt-actors-cluster/dnssd-v2/api/
    RUN cd goakt-actors-cluster/dnssd-v2/api && oapi-codegen -config cfg.yaml openapi.yaml

    SAVE ARTIFACT goakt-actors-cluster/dnssd-v2/api/api.gen.go AS LOCAL goakt-actors-cluster/dnssd-v2/api/

opengen-k8s-v2:
    WORKDIR /app

    COPY goakt-actors-cluster/k8s-v2/api/openapi.yaml goakt-actors-cluster/k8s-v2/api/cfg.yaml goakt-actors-cluster/k8s-v2/api/
    RUN cd goakt-actors-cluster/k8s-v2/api && oapi-codegen -config cfg.yaml openapi.yaml

    SAVE ARTIFACT goakt-actors-cluster/k8s-v2/api/api.gen.go AS LOCAL goakt-actors-cluster/k8s-v2/api/

opengen-saga:
    WORKDIR /app

    COPY goakt-saga/api/openapi.yaml goakt-saga/api/cfg.yaml goakt-saga/api/
    RUN cd goakt-saga/api && oapi-codegen -config cfg.yaml openapi.yaml

    SAVE ARTIFACT goakt-saga/api/api.gen.go AS LOCAL goakt-saga/api/

compile-goakt-ai:
    COPY +vendor/files ./

    RUN go build -mod=vendor -o bin/goakt-ai ./goakt-ai
    SAVE ARTIFACT bin/goakt-ai /goakt-ai

goakt-ai-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-goakt-ai/goakt-ai ./goakt-ai
    RUN chmod +x ./goakt-ai

    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320

    ENTRYPOINT ["./goakt-ai"]
    SAVE IMAGE goakt-ai:dev

compile-k8s:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./goakt-actors-cluster/k8s
    SAVE ARTIFACT bin/accounts /accounts

k8s-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-k8s/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev-k8s

compile-k8s-v2:
    COPY +vendor/files ./

    RUN go build -mod=vendor -o bin/accounts ./goakt-actors-cluster/k8s-v2
    SAVE ARTIFACT bin/accounts /accounts

k8s-v2-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-k8s-v2/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev-k8s-v2

compile-saga:
    COPY +vendor/files ./

    RUN go build -mod=vendor -o bin/saga-transfer ./goakt-saga
    SAVE ARTIFACT bin/saga-transfer /saga-transfer

saga-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-saga/saga-transfer ./saga-transfer
    RUN chmod +x ./saga-transfer

    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320

    ENTRYPOINT ["./saga-transfer"]
    SAVE IMAGE saga-transfer:dev

compile-two-pc:
    COPY +vendor/files ./

    RUN go build -mod=vendor -o bin/two-pc-transfer ./goakt-2pc
    SAVE ARTIFACT bin/two-pc-transfer /two-pc-transfer

two-pc-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-two-pc/two-pc-transfer ./two-pc-transfer
    RUN chmod +x ./two-pc-transfer

    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320

    ENTRYPOINT ["./two-pc-transfer"]
    SAVE IMAGE two-pc-transfer:dev

compile-k8s-ebpf:
    COPY +vendor/files ./

    RUN go build -mod=vendor -o bin/accounts ./goakt-actors-cluster/k8s-ebpf
    SAVE ARTIFACT bin/accounts /accounts

k8s-ebpf-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-k8s-ebpf/accounts ./accounts
    RUN chmod +x ./accounts

    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev-k8s-ebpf

compile-dnssd:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./goakt-actors-cluster/dnssd
    SAVE ARTIFACT bin/accounts /accounts

dnssd-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-dnssd/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320
    EXPOSE 9092

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev

compile-static:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./goakt-actors-cluster/static
    SAVE ARTIFACT bin/accounts /accounts

static-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-static/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320
    EXPOSE 9092

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev

compile-grains-dnssd:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./goakt-grains-cluster/grains-dnssd
    SAVE ARTIFACT bin/accounts /accounts

dnssd-grains-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-grains-dnssd/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320
    EXPOSE 9092

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts-grains:dev


compile-dnssd-v2:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./goakt-actors-cluster/dnssd-v2
    SAVE ARTIFACT bin/accounts /accounts

dnssd-v2-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-dnssd-v2/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320
    EXPOSE 9092

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev

compile-dynalloc:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./goakt-actors-cluster/dynalloc
    SAVE ARTIFACT bin/accounts /accounts

dynalloc-image:
    FROM alpine:3.17

    WORKDIR /app
    COPY +compile-dynalloc/accounts ./accounts
    RUN chmod +x ./accounts

    # expose the various ports in the container
    EXPOSE 50051
    EXPOSE 50052
    EXPOSE 3322
    EXPOSE 3320
    EXPOSE 9092

    ENTRYPOINT ["./accounts"]
    SAVE IMAGE accounts:dev-dynalloc
