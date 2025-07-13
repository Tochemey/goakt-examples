.PHONY: ingress
ingress:
	minikube addons enable ingress

.PHONY: cluster-down
cluster-down:
	kubectl delete -f ./nginx-config.yaml
	kubectl delete -f ./nginx-deployment.yaml
	kubectl delete -f ./nginx-service.yaml
	kubectl delete -f ./k8s.yaml

.PHONY: cluster-up
cluster-up:
	minikube image load --overwrite=true accounts:dev-k8s
	kubectl apply -f ./nginx-config.yaml
	kubectl apply -f ./nginx-deployment.yaml
	kubectl apply -f ./nginx-service.yaml
	kubectl apply -f ./k8s.yaml

.PHONY: host
host:
	minikube ip