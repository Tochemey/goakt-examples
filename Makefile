# Docker-based build for the GoAkt examples.
#
# Code generation (protobuf, Connect, OpenAPI) runs inside a builder image
# (Dockerfile.build), and the example images are built with the shared
# multi-stage Dockerfile at the root of the repository. The only requirement
# is Docker: https://docs.docker.com/get-docker/

.DEFAULT_GOAL := help

BUILDER_IMAGE := goakt-examples-builder

DOCKER_RUN := docker run --rm -v "$(CURDIR)":/app -w /app $(BUILDER_IMAGE)

# $(call example-image,<example dir>,<binary name>,<image tag>)
define example-image
docker build --build-arg EXAMPLE=$(1) --build-arg BINARY=$(2) -t $(3) .
endef

.PHONY: help
help:
	@echo "GoAkt Examples"
	@echo ""
	@echo "Targets:"
	@echo "  build                   Compile every example"
	@echo "  vendor                  go mod tidy && go mod vendor"
	@echo "  all                     Run every code generation target"
	@echo "  protogen                Generate the protobuf/Connect code (internal/*pb)"
	@echo "  opengen                 Generate the dnssd OpenAPI code"
	@echo "  opengen-k8s             Generate the k8s OpenAPI code"
	@echo "  opengen-saga            Generate the saga OpenAPI code"
	@echo "  opengen-multi-dc        Generate the multi-dc OpenAPI code"
	@echo "  images                  Build every example Docker image"
	@echo "  dnssd-image             Build accounts:dev from goakt-cluster/dnssd"
	@echo "  static-image            Build accounts:dev from goakt-cluster/static"
	@echo "  dynalloc-image          Build accounts:dev-dynalloc"
	@echo "  k8s-image               Build accounts:dev-k8s"
	@echo "  k8s-ebpf-image          Build accounts:dev-k8s-ebpf"
	@echo "  multi-dc-image          Build accounts:dev-multi-dc"
	@echo "  multi-dc-isolated-image Build accounts:dev-multi-dc-isolated"
	@echo "  dnssd-grains-image      Build accounts-grains:dev"
	@echo "  goakt-ai-image          Build goakt-ai:dev"
	@echo "  saga-image              Build saga-transfer:dev"
	@echo "  two-pc-image            Build two-pc-transfer:dev"
	@echo "  blockchain-image        Build blockchain:dev"

# goakt-grains-cluster/grains-dnssd is excluded: the vendored
# github.com/tochemey/gopack v0.2.1 postgres testkit does not compile against
# the vendored moby/moby API v1.55.0. Put it back once a gopack release fixes it.
.PHONY: build
build:
	go build -mod=vendor $$(go list ./... | grep -v goakt-grains-cluster/grains-dnssd)

.PHONY: vendor
vendor:
	go mod tidy && go mod vendor

.PHONY: all
all: protogen opengen opengen-k8s opengen-saga opengen-multi-dc

.PHONY: builder
builder:
	docker build -f Dockerfile.build -t $(BUILDER_IMAGE) .

.PHONY: protogen
protogen: builder
	$(DOCKER_RUN) buf generate \
		--template buf.gen.yaml \
		--path protos/sample \
		--path protos/helloworld \
		--path protos/chat
	rm -rf internal/samplepb internal/chatpb internal/helloworldpb
	cp -R gen/sample internal/samplepb
	cp -R gen/chat internal/chatpb
	cp -R gen/helloworld internal/helloworldpb
	rm -rf gen

.PHONY: opengen
opengen: builder
	$(DOCKER_RUN) sh -c "cd goakt-cluster/dnssd/api && oapi-codegen -config cfg.yaml openapi.yaml"

.PHONY: opengen-k8s
opengen-k8s: builder
	$(DOCKER_RUN) sh -c "cd goakt-cluster/k8s/api && oapi-codegen -config cfg.yaml openapi.yaml"

.PHONY: opengen-saga
opengen-saga: builder
	$(DOCKER_RUN) sh -c "cd goakt-saga/api && oapi-codegen -config cfg.yaml openapi.yaml"

.PHONY: opengen-multi-dc
opengen-multi-dc: builder
	$(DOCKER_RUN) sh -c "cd goakt-cluster/multi-dc/api && oapi-codegen -config cfg.yaml openapi.yaml"

.PHONY: images
images: dnssd-image dynalloc-image k8s-image k8s-ebpf-image multi-dc-image multi-dc-isolated-image dnssd-grains-image goakt-ai-image saga-image two-pc-image blockchain-image

.PHONY: dnssd-image
dnssd-image:
	$(call example-image,goakt-cluster/dnssd,accounts,accounts:dev)

# the static example runs the same accounts:dev image as dnssd
.PHONY: static-image
static-image:
	$(call example-image,goakt-cluster/static,accounts,accounts:dev)

.PHONY: dynalloc-image
dynalloc-image:
	$(call example-image,goakt-cluster/dynalloc,accounts,accounts:dev-dynalloc)

.PHONY: k8s-image
k8s-image:
	$(call example-image,goakt-cluster/k8s,accounts,accounts:dev-k8s)

.PHONY: k8s-ebpf-image
k8s-ebpf-image:
	$(call example-image,goakt-cluster/k8s-ebpf,accounts,accounts:dev-k8s-ebpf)

.PHONY: multi-dc-image
multi-dc-image:
	$(call example-image,goakt-cluster/multi-dc,accounts,accounts:dev-multi-dc)

.PHONY: multi-dc-isolated-image
multi-dc-isolated-image:
	$(call example-image,goakt-cluster/multi-dc-isolated,accounts,accounts:dev-multi-dc-isolated)

.PHONY: dnssd-grains-image
dnssd-grains-image:
	$(call example-image,goakt-grains-cluster/grains-dnssd,accounts,accounts-grains:dev)

.PHONY: goakt-ai-image
goakt-ai-image:
	$(call example-image,goakt-ai,goakt-ai,goakt-ai:dev)

.PHONY: saga-image
saga-image:
	$(call example-image,goakt-saga,saga-transfer,saga-transfer:dev)

.PHONY: two-pc-image
two-pc-image:
	$(call example-image,goakt-2pc,two-pc-transfer,two-pc-transfer:dev)

.PHONY: blockchain-image
blockchain-image:
	$(call example-image,goakt-blockchain,blockchain,blockchain:dev)
