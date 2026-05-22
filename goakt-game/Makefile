# goakt-game Makefile
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
#   make build              compile the binary into ./bin/goakt-game
#   make clean              remove the build output
#
# Open http://localhost:8080 (node A) or http://localhost:8081 (node B)
# after the cluster is up — the matchmaker singleton silently distributes
# matches across both regardless of which one your browser connects to.

.PHONY: build run web cluster-up cluster-down cluster-logs local-node-a local-node-b local-stop clean

BIN      := bin/goakt-game
BIN_DIR  := bin

# TypeScript source → committed JS output. `make web` regenerates the JS
# (requires Node.js + npx). The compiled file is committed so `go run`,
# `make build` and `docker compose` work without Node installed.
TS_SRC   := web/main.ts
JS_OUT   := web/main.js
TS_CFG   := tsconfig.json

# Single-process cluster ports — used by `local-node-a` / `local-node-b`.
A_HTTP   := 8080
A_REMOTE := 9000
A_DISCO  := 9001
A_PEER   := 9002

B_HTTP   := 8081
B_REMOTE := 9100
B_DISCO  := 9101
B_PEER   := 9102

SEEDS    := 127.0.0.1:$(A_DISCO),127.0.0.1:$(B_DISCO)

# Ports that `local-stop` should free.
PORTS    := $(A_HTTP) $(A_REMOTE) $(A_DISCO) $(A_PEER) $(B_HTTP) $(B_REMOTE) $(B_DISCO) $(B_PEER)

# Docker compose CLI — supports both `docker compose` (v2) and legacy
# `docker-compose` (v1). Override with `make COMPOSE=...` if needed.
COMPOSE ?= docker compose

# ---------------------------------------------------------------- build

# Compile the TypeScript client. The file-level Make dependency makes
# this a no-op when neither the source nor the tsconfig has changed.
web: $(JS_OUT)

$(JS_OUT): $(TS_SRC) $(TS_CFG)
	npx --package=typescript@5.6 -y -- tsc -p .

# `go build` embeds web/main.js via //go:embed, so the JS *must* exist
# before we hand off to the Go compiler.
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
	$(COMPOSE) down

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

# Kill anything holding the example's ports — useful after a CTRL-C
# that didn't fully release a socket or after a crashed run.
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
