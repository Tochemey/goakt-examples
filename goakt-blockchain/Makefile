# Makefile for the blockchain example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Docker: https://docs.docker.com/get-docker/ (for building the image)
#
# Quick start:
#   make cluster-create
#   make deploy
#   make port-forward   # in another terminal
#   make test
#
# Run from: goakt-blockchain/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-blockchain
IMAGE_NAME ?= blockchain:dev
K8S_DIR := ./deploy
REPO_ROOT := ..

.PHONY: help
help:
	@echo "Blockchain GoAkt Example - Kind (Kubernetes in Docker)"
	@echo ""
	@echo "Targets:"
	@echo "  deploy           Build image, load into Kind, and deploy (recommended)"
	@echo "  cluster-create   Create a new Kind cluster"
	@echo "  cluster-recreate Delete and recreate the cluster"
	@echo "  cluster-delete   Delete the Kind cluster"
	@echo "  image            Build the Docker image and load into Kind"
	@echo "  cluster-up       Deploy the blockchain pods and nginx"
	@echo "  cluster-down     Remove all deployments"
	@echo "  status           Show cluster and pod status"
	@echo "  test             Run the API integration test"
	@echo "  test-resilience  Kill the pod hosting the node singleton and verify failover"
	@echo "  logs             Tail logs from the blockchain pods"
	@echo "  port-forward     Forward nginx service to localhost:8080"

.PHONY: cluster-create
cluster-create:
	@echo "Creating Kind cluster '$(CLUSTER_NAME)'..."
	kind create cluster --name $(CLUSTER_NAME) --config $(K8S_DIR)/kind-config.yaml --wait 2m
	@echo "Kind cluster created. Use 'make deploy' to deploy."

.PHONY: cluster-delete
cluster-delete:
	@echo "Deleting Kind cluster '$(CLUSTER_NAME)'..."
	kind delete cluster --name $(CLUSTER_NAME)
	@echo "Kind cluster deleted."

.PHONY: cluster-recreate
cluster-recreate: cluster-delete cluster-create
	@echo "Cluster recreated."

.PHONY: image
image:
	@echo "Building Docker image..."
	$(MAKE) -C $(REPO_ROOT) blockchain-image
	@echo "Loading image into Kind cluster..."
	kind load docker-image $(IMAGE_NAME) --name $(CLUSTER_NAME)
	@echo "Image loaded successfully."

.PHONY: deploy
deploy: image cluster-up

.PHONY: cluster-up
cluster-up:
	@echo "Deploying blockchain and nginx..."
	kubectl apply -f $(K8S_DIR)/nginx-config.yaml -f $(K8S_DIR)/nginx-deployment.yaml -f $(K8S_DIR)/nginx-service.yaml
	kubectl apply -f $(K8S_DIR)/k8s.yaml
	kubectl rollout status statefulset/blockchain --timeout=180s
	kubectl rollout status deployment/nginx --timeout=120s
	@echo "Deployment complete. Use 'make port-forward' then 'make test'."

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/k8s.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml -f $(K8S_DIR)/nginx-deployment.yaml -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete pvc -l app.kubernetes.io/name=blockchain --ignore-not-found
	@echo "Deployments removed."

.PHONY: status
status:
	kind get clusters
	kubectl get pods -o wide
	kubectl get services

.PHONY: test
test:
	./scripts/test-api.sh

.PHONY: test-resilience
test-resilience:
	./scripts/test-resilience.sh

.PHONY: logs
logs:
	kubectl logs -l app.kubernetes.io/name=blockchain -f --tail=100 --prefix

.PHONY: port-forward
port-forward:
	@echo "Forwarding nginx to http://localhost:8080 (Ctrl+C to stop)..."
	kubectl port-forward svc/nginx 8080:80
