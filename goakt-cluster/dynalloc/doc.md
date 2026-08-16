# How to run it?

Dynalloc example show to create actors with location transparency across a cluster of nodes

1. install [Docker](https://docs.docker.com/get-docker/)
2. clone the repository
3. run at the root of the cloned repository `make dynalloc-image` (builds `accounts:dev-dynalloc`)
4. run `docker-compose up -d` to start the cluster of three nodes. To stop the cluster just run
   `docker-compose down -v --remove-orphans`
5. run `docker-compose ps` to list the running instances and their exposed ports to use by any grpc client. With any
   gRPC client you can access the service. The service
   definitions is [here](../../../protos/sample/pb/v1)
