VERSION 0.8

FROM tochemey/docker-go:1.25.1-5.3.0

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