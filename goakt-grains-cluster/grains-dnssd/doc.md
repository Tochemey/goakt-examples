# How to run it?

1. install [Earthly](https://earthly.dev/get-earthly)
2. clone the repository
3. run at the root of the cloned repository `earthly +dnssd-grains-image`
4. run `docker-compose up` to start the cluster of three nodes behind Nginx. To stop the cluster just run `docker-compose down -v --remove-orphans`
5. run `docker-compose ps` to list the running instances. Use `localhost:8000` as the stable gRPC endpoint. The service
   definitions is [here](../../../protos/sample/pb/v1)
