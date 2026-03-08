# Makefile for goakt-2pc example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Earthly: https://earthly.dev/get-earthly (for building the image)
#
# Quick start:
#   make cluster-create    # Create Kind cluster (one-time)
#   make image             # Build and load image into Kind
#   make cluster-up       # Deploy all components
#   make test             # Run API tests
#   make cluster-down     # Tear down deployments
#
# Run from: goakt-2pc/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-2pc
IMAGE_NAME ?= 2pc-transfer:dev
K8S_DIR := ./k8s
REPO_ROOT := ..

.PHONY: help
help:
	@echo "goakt-2pc - 2pc-based Money Transfer (Kind/Kubernetes)"
	@echo ""
	@echo "Targets:"
	@echo "  deploy           Build image, load into Kind, and deploy (recommended)"
	@echo "  cluster-create   Create a new Kind cluster"
	@echo "  cluster-recreate Delete and recreate the cluster"
	@echo "  cluster-delete   Delete the Kind cluster"
	@echo "  image            Build the Docker image and load into Kind"
	@echo "  cluster-up       Deploy PostgreSQL, 2pc-transfer, and nginx"
	@echo "  cluster-down     Remove all deployments"
	@echo "  status           Show cluster and pod status"
	@echo "  test             Run API integration tests"
	@echo "  logs             Tail logs from 2pc-transfer pods"
	@echo "  port-forward     Forward nginx service to localhost:8080"
	@echo "  port-forward-jaeger  Forward Jaeger UI to localhost:16686"

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
	cd $(REPO_ROOT) && earthly +2pc-image
	@echo "Loading image into Kind cluster..."
	kind load docker-image $(IMAGE_NAME) --name $(CLUSTER_NAME)
	@echo "Image loaded successfully."

.PHONY: deploy
deploy: image cluster-up

.PHONY: cluster-up
cluster-up:
	@echo "Deploying PostgreSQL..."
	kubectl apply -f $(K8S_DIR)/postgres-secret.yaml
	kubectl apply -f $(K8S_DIR)/postgres-configmap.yaml
	kubectl apply -f $(K8S_DIR)/postgres-deployment.yaml
	@echo "Waiting for PostgreSQL to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/postgres
	@echo "Deploying tracing stack (Jaeger + OTEL Collector)..."
	kubectl apply -f $(K8S_DIR)/jaeger-deployment.yaml
	kubectl create configmap otel-collector-config --from-file=config.yaml=$(K8S_DIR)/otel-collector-config.yaml --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f $(K8S_DIR)/otel-collector-deployment.yaml
	@echo "Waiting for OTEL Collector to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/otel-collector
	@echo "Deploying 2pc-transfer service..."
	kubectl apply -f $(K8S_DIR)/k8s.yaml
	@echo "Waiting for 2pc-transfer pods to be ready..."
	@for i in 0 1 2; do \
		echo "  Waiting for 2pc-transfer-$$i..."; \
		elapsed=0; \
		until [ "$$(kubectl get pod 2pc-transfer-$$i -o jsonpath='{.status.containerStatuses[?(@.name=="2pc-transfer")].ready}' 2>/dev/null)" = "true" ]; do \
			sleep 5; elapsed=$$((elapsed+5)); \
			if [ $$elapsed -ge 300 ]; then \
				echo "  Timeout waiting for 2pc-transfer-$$i."; \
				kubectl get pod 2pc-transfer-$$i -o wide 2>/dev/null; \
				exit 1; \
			fi; \
		done; \
		echo "  2pc-transfer-$$i is ready"; \
	done
	@echo "Deploying nginx ingress..."
	kubectl apply -f $(K8S_DIR)/nginx-config.yaml
	kubectl apply -f $(K8S_DIR)/nginx-deployment.yaml
	kubectl apply -f $(K8S_DIR)/nginx-service.yaml
	@echo "All components deployed. Run 'make port-forward' to access the API."

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/k8s.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/otel-collector-deployment.yaml --ignore-not-found
	kubectl delete configmap otel-collector-config --ignore-not-found
	kubectl delete -f $(K8S_DIR)/jaeger-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-configmap.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-secret.yaml --ignore-not-found
	@echo "Deployments removed."

.PHONY: status
status:
	@echo "=== Cluster Info ==="
	kubectl cluster-info --context kind-$(CLUSTER_NAME)
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -o wide
	@echo ""
	@echo "=== Services ==="
	kubectl get svc

.PHONY: logs
logs:
	kubectl logs -l app.kubernetes.io/name=2pc-transfer -f --tail=100

.PHONY: port-forward
port-forward:
	@echo "Waiting for nginx to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/nginx
	@echo "Forwarding nginx (port 80) to localhost:8080..."
	@echo "API available at http://localhost:8080"
	@echo "Swagger UI at http://localhost:8080/docs"
	kubectl port-forward service/nginx 8080:80

.PHONY: port-forward-jaeger
port-forward-jaeger:
	@echo "Forwarding Jaeger UI to localhost:16686..."
	@echo "Jaeger UI at http://localhost:16686"
	kubectl port-forward service/jaeger-collector 16686:16686

.PHONY: test
test:
	@echo "Running API tests..."
	@chmod +x scripts/test-api.sh 2>/dev/null || true
	@sh ./scripts/test-api.sh || true
