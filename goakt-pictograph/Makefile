# goakt-pictograph Makefile
#
# Single-node (local process):
#   make run                run on default ports (open http://localhost:8080)
#
# Cluster mode (docker compose — canonical):
#   make cluster-up         build images and start node-a + node-b
#   make cluster-logs       follow both nodes' logs
#   make cluster-down       stop and remove containers + network
#
# Cluster mode (local processes — for fast dev iteration):
#   make local-node-a       first node, in one terminal
#   make local-node-b       second node, in another terminal
#   make local-stop         kill anything bound to the example's ports
#
# Other:
#   make build              compile the binary into ./bin/goakt-pictograph
#   make clean              remove the build output

.PHONY: build run web cluster-up cluster-down cluster-logs local-node-a local-node-b local-stop clean

BIN      := bin/goakt-pictograph
BIN_DIR  := bin

TS_SRC   := web/main.ts
JS_OUT   := web/main.js
TS_CFG   := tsconfig.json

A_HTTP   := 8080
A_REMOTE := 9000
A_DISCO  := 9001
A_PEER   := 9002

B_HTTP   := 8081
B_REMOTE := 9100
B_DISCO  := 9101
B_PEER   := 9102

SEEDS    := 127.0.0.1:$(A_DISCO),127.0.0.1:$(B_DISCO)

PORTS    := $(A_HTTP) $(A_REMOTE) $(A_DISCO) $(A_PEER) $(B_HTTP) $(B_REMOTE) $(B_DISCO) $(B_PEER)

COMPOSE ?= docker compose

# ---------------------------------------------------------------- build

web: $(JS_OUT)

$(JS_OUT): $(TS_SRC) $(TS_CFG)
	npx --package=typescript@5.6 -y -- tsc -p .

build: web
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) .

run: build
	$(BIN)

# ---------------------------------------------------------------- docker compose cluster

cluster-up:
	$(COMPOSE) up --build -d
	@echo
	@echo "  node-a → http://localhost:8080"
	@echo "  node-b → http://localhost:8081"
	@echo
	@echo "  follow logs:    make cluster-logs"
	@echo "  tear down:      make cluster-down"

cluster-logs:
	$(COMPOSE) logs -f --tail=100

cluster-down:
	$(COMPOSE) down --volumes --remove-orphans

# ---------------------------------------------------------------- local-process cluster (dev)

local-node-a: build
	$(BIN) \
		--http-port      $(A_HTTP) \
		--remoting-port  $(A_REMOTE) \
		--discovery-port $(A_DISCO) \
		--peers-port     $(A_PEER) \
		--peers          "$(SEEDS)"

local-node-b: build
	$(BIN) \
		--http-port      $(B_HTTP) \
		--remoting-port  $(B_REMOTE) \
		--discovery-port $(B_DISCO) \
		--peers-port     $(B_PEER) \
		--peers          "$(SEEDS)"

local-stop:
	@for p in $(PORTS); do \
		pid=$$(lsof -ti :$$p 2>/dev/null); \
		if [ -n "$$pid" ]; then \
			echo "kill -9 $$pid (port $$p)"; \
			kill -9 $$pid 2>/dev/null || true; \
		fi; \
	done

# ---------------------------------------------------------------- clean

clean:
	rm -rf $(BIN_DIR)
