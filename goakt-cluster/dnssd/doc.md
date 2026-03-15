# How to run it?

1. install [Earthly](https://earthly.dev/get-earthly)
2. clone the repository
3. run at the root of the cloned repository `earthly +dnssd-image`
4. run `docker compose up -d tracer prometheus collector db`
5. run `docker compose up -d lb coredns lb accounts1 accounts2 accounts3` to start the cluster.
6. run `docker-compose ps` to list the running instances and their exposed ports to use by any grpc client. With any
   gRPC client you can access the service. The service
   definitions is [here](../../../protos/sample/pb/v1)
7. To stop the cluster just run `docker compose down -v --remove-orphans`
