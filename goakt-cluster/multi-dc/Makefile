# Makefile for multi-dc example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Earthly: https://earthly.dev/get-earthly (for building the image)
#
# Quick start:
#   make cluster-create    # Create Kind cluster (one-time)
#   make deploy            # Build and deploy all components
#   make port-forward      # Expose both DCs (DC-1 on :8080, DC-2 on :8081)
#   make test              # Run cross-DC integration tests
#
# Run from: goakt-cluster/multi-dc/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-multi-dc
IMAGE_NAME ?= accounts:dev-multi-dc
K8S_DIR := ./deploy
REPO_ROOT := ../..

.PHONY: help
help:
	@echo "multi-dc GoAkt Example - Multi-Datacenter with NATS Discovery"
	@echo ""
	@echo "Targets:"
	@echo "  deploy           Build image, load into Kind, and deploy (recommended)"
	@echo "  cluster-create   Create a new Kind cluster"
	@echo "  cluster-recreate Delete and recreate the cluster"
	@echo "  cluster-delete   Delete the Kind cluster"
	@echo "  image            Build the Docker image and load into Kind"
	@echo "  cluster-up       Deploy NATS, PostgreSQL (x2), accounts (DC-1 + DC-2), and nginx"
	@echo "  cluster-down     Remove all deployments"
	@echo "  status           Show cluster and pod status"
	@echo "  test             Run cross-DC API integration tests"
	@echo "  logs-dc1         Tail logs from DC-1 pods"
	@echo "  logs-dc2         Tail logs from DC-2 pods"
	@echo "  port-forward     Forward nginx to localhost:8080 (DC-1 and DC-2 via /dc1/ and /dc2/)"
	@echo "  port-forward-jaeger  Forward Jaeger UI to localhost:16686 (traces)"
	@echo "  dashboard        Install (if needed) and access Kubernetes dashboard (Headlamp)"
	@echo "  k9s              Launch k9s terminal dashboard"

.PHONY: cluster-create
cluster-create:
	@echo "Creating Kind cluster '$(CLUSTER_NAME)'..."
	kind create cluster --name $(CLUSTER_NAME) --config $(K8S_DIR)/kind-config.yaml --wait 2m
	@echo "Installing metrics-server..."
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	kubectl patch deployment metrics-server -n kube-system --type='json' \
		-p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
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
	cd $(REPO_ROOT) && earthly +multi-dc-image
	@echo "Loading image into Kind cluster..."
	kind load docker-image $(IMAGE_NAME) --name $(CLUSTER_NAME)
	@echo "Image loaded successfully."

.PHONY: deploy
deploy: image cluster-up

.PHONY: cluster-up
cluster-up:
	@echo "Deploying NATS (JetStream)..."
	kubectl apply -f $(K8S_DIR)/nats-deployment.yaml
	@echo "Waiting for NATS to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/nats
	@echo "Deploying PostgreSQL (DC-1 and DC-2)..."
	kubectl apply -f $(K8S_DIR)/postgres-secret.yaml
	kubectl apply -f $(K8S_DIR)/postgres-configmap.yaml
	kubectl apply -f $(K8S_DIR)/postgres-dc1-deployment.yaml
	kubectl apply -f $(K8S_DIR)/postgres-dc2-deployment.yaml
	@echo "Waiting for PostgreSQL instances to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/postgres-dc1
	kubectl wait --for=condition=available --timeout=120s deployment/postgres-dc2
	@echo "Deploying tracing stack (Jaeger + OTEL Collector)..."
	kubectl apply -f $(K8S_DIR)/jaeger-deployment.yaml
	kubectl create configmap otel-collector-config --from-file=config.yaml=$(K8S_DIR)/otel-collector-config.yaml --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f $(K8S_DIR)/otel-collector-deployment.yaml
	@echo "Waiting for OTEL Collector to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/otel-collector
	@echo "Deploying DC-1 accounts service (3 replicas)..."
	kubectl apply -f $(K8S_DIR)/dc1.yaml
	@echo "Deploying DC-2 accounts service (3 replicas)..."
	kubectl apply -f $(K8S_DIR)/dc2.yaml
	@echo "Waiting for DC-1 pods to be ready..."
	@for i in 0 1 2; do \
		echo "  Waiting for accounts-dc1-$$i..."; \
		elapsed=0; \
		until [ "$$(kubectl get pod accounts-dc1-$$i -o jsonpath='{.status.containerStatuses[?(@.name=="accounts")].ready}' 2>/dev/null)" = "true" ]; do \
			sleep 5; elapsed=$$((elapsed+5)); \
			if [ $$elapsed -ge 300 ]; then \
				echo "  Timeout waiting for accounts-dc1-$$i."; \
				kubectl describe pod accounts-dc1-$$i 2>/dev/null | tail -20; \
				exit 1; \
			fi; \
		done; \
		echo "  accounts-dc1-$$i is ready"; \
	done
	@echo "Waiting for DC-2 pods to be ready..."
	@for i in 0 1 2; do \
		echo "  Waiting for accounts-dc2-$$i..."; \
		elapsed=0; \
		until [ "$$(kubectl get pod accounts-dc2-$$i -o jsonpath='{.status.containerStatuses[?(@.name=="accounts")].ready}' 2>/dev/null)" = "true" ]; do \
			sleep 5; elapsed=$$((elapsed+5)); \
			if [ $$elapsed -ge 300 ]; then \
				echo "  Timeout waiting for accounts-dc2-$$i."; \
				kubectl describe pod accounts-dc2-$$i 2>/dev/null | tail -20; \
				exit 1; \
			fi; \
		done; \
		echo "  accounts-dc2-$$i is ready"; \
	done
	@echo "Deploying nginx ingress..."
	kubectl apply -f $(K8S_DIR)/nginx-config.yaml
	kubectl apply -f $(K8S_DIR)/nginx-deployment.yaml
	kubectl apply -f $(K8S_DIR)/nginx-service.yaml
	@echo "All components deployed."
	@echo "  DC-1: 3 replicas (us-east-1)"
	@echo "  DC-2: 3 replicas (eu-west-1)"
	@echo "Run 'make port-forward' to access the API."

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/dc2.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/dc1.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/otel-collector-deployment.yaml --ignore-not-found
	kubectl delete configmap otel-collector-config --ignore-not-found
	kubectl delete -f $(K8S_DIR)/jaeger-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-dc2-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-dc1-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-configmap.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-secret.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nats-deployment.yaml --ignore-not-found
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

.PHONY: logs-dc1
logs-dc1:
	kubectl logs -l dc=dc-1 -f --tail=100

.PHONY: logs-dc2
logs-dc2:
	kubectl logs -l dc=dc-2 -f --tail=100

.PHONY: port-forward
port-forward:
	@echo "Waiting for nginx to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/nginx
	@echo "Forwarding nginx to localhost:8080..."
	@echo "  DC-1 API: http://localhost:8080/dc1/"
	@echo "  DC-2 API: http://localhost:8080/dc2/"
	@echo "  Default (DC-1): http://localhost:8080/"
	@echo "  Swagger UI: http://localhost:8080/docs"
	kubectl port-forward service/nginx 8080:8080

.PHONY: port-forward-jaeger
port-forward-jaeger:
	@echo "Forwarding Jaeger UI to localhost:16686..."
	@echo "Jaeger UI at http://localhost:16686"
	kubectl port-forward service/jaeger-collector 16686:16686

.PHONY: test
test:
	@echo "Running cross-DC API tests..."
	@chmod +x scripts/test-api.sh 2>/dev/null || true
	@sh ./scripts/test-api.sh || true

.PHONY: dashboard-install
dashboard-install:
	@echo "Installing Headlamp (Kubernetes dashboard)..."
	helm upgrade --install headlamp headlamp \
		--repo https://kubernetes-sigs.github.io/headlamp \
		--namespace headlamp --create-namespace \
		--wait --timeout 120s
	@echo "Creating dashboard admin user..."
	-kubectl create serviceaccount headlamp-admin -n headlamp 2>/dev/null || true
	-kubectl create clusterrolebinding headlamp-admin-binding --clusterrole=cluster-admin --serviceaccount=headlamp:headlamp-admin 2>/dev/null || true
	@echo "Dashboard installed. Run 'make dashboard' to access it."

.PHONY: dashboard
dashboard: dashboard-install
	@echo ""
	@echo "=========================================="
	@echo "Kubernetes Dashboard (Headlamp)"
	@echo "=========================================="
	@echo "URL: http://localhost:4466"
	@echo ""
	@echo "Login token (copy and paste at the dashboard login):"
	@kubectl -n headlamp create token headlamp-admin
	@echo ""
	@echo "Port-forwarding dashboard (Ctrl+C to stop)..."
	kubectl -n headlamp port-forward svc/headlamp 4466:80

.PHONY: k9s
k9s:
	@command -v k9s >/dev/null 2>&1 || { echo "k9s not found. Install with: brew install k9s"; exit 1; }
	k9s --context kind-$(CLUSTER_NAME)
