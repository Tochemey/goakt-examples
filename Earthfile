VERSION 0.8

FROM golang:1.25.3-alpine

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

# install the various tools to generate connect-go
RUN go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
RUN go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

all:
	BUILD +protogen

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
            --path protos/bench \
            --path protos/helloworld \
            --path protos/chat

    # save artifact to
    SAVE ARTIFACT gen/sample AS LOCAL internal/samplepb
    SAVE ARTIFACT gen/chat AS LOCAL internal/chatpb
    SAVE ARTIFACT gen/bench AS LOCAL internal/benchpb
    SAVE ARTIFACT gen/helloworld AS LOCAL internal/helloworldpb

compile-k8s:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./actor-cluster/k8s
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


compile-dnssd:
    COPY +vendor/files ./

    RUN go build -mod=vendor  -o bin/accounts ./actor-cluster/dnssd
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

    RUN go build -mod=vendor  -o bin/accounts ./actor-cluster/static
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

    RUN go build -mod=vendor  -o bin/accounts ./grains-cluster/grains-dnssd
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