# How to run it?

1. install [Earthly](https://earthly.dev/get-earthly)
2. install [Minikube](https://minikube.sigs.k8s.io/docs/start/)
3. run `minikube start`
4. run `minikube addons enable ingress` to enable the ingress controller
5. run `minikube addons enable metrics-server`
6. run `minikube tunnel`
7. clone the repository
8. run at the root of the cloned repository `earthly +k8s-image`
9. run `make cluster-up` to start the cluster. To stop the cluster just run `make cluster-down`
10. run `make host` to get the host IP address of the cluster IP
11. run `kubectl port-forward service/nginx 8080:80`. With any gRPC client you can access the service. The service
    definitions is [here](../../../protos/sample/pb/v1)
