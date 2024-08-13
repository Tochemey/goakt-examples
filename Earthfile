VERSION 0.8

FROM tochemey/docker-go:1.22.2-3.1.0

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
    COPY buf.work.yaml buf.gen.yaml ./

    # generate the pbs
    RUN buf generate \
            --template buf.gen.yaml \
            --path protos/benchmark \
            --path protos/sample

    # save artifact to
    SAVE ARTIFACT gen/sample AS LOCAL samplepb
    SAVE ARTIFACT gen/benchmark AS LOCAL bench/benchmarkpb

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
    SAVE IMAGE accounts:dev


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
