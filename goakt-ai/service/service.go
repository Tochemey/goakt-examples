// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
)

const askTimeout = 60 * time.Second

// QueryRequest is the HTTP request body for POST /query
type QueryRequest struct {
	Query string `json:"query"`
}

// QueryResponse is the HTTP response for POST /query
type QueryResponse struct {
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

// QueryService serves the minimal query HTTP endpoint for the CLI.
// Each node spawns its own OrchestratorAgent (named with the node
// hostname to avoid cluster-wide name collisions). The orchestrator
// delegates to specialized agents that may live on any node.
type QueryService struct {
	actorSystem     goakt.ActorSystem
	orchestratorPID *goakt.PID
	logger          log.Logger
	port            int
	nodeName        string
	server          *http.Server
	tracerProvider  trace.TracerProvider
}

// NewQueryService creates a new query service.
// nodeName must be unique per cluster node (e.g. the pod hostname).
func NewQueryService(system goakt.ActorSystem, port int, nodeName string, logger log.Logger, tp trace.TracerProvider) *QueryService {
	return &QueryService{
		actorSystem:    system,
		logger:         logger,
		port:           port,
		nodeName:       nodeName,
		tracerProvider: tp,
	}
}

// Start spawns a node-local orchestrator and starts the HTTP server.
// It retries the spawn to handle the case where a rolling restart causes
// a brief overlap: the cluster may still hold a stale actor entry from
// the previous pod incarnation until it detects the old node is gone.
func (s *QueryService) Start(ctx context.Context) error {
	orchestratorName := fmt.Sprintf("orchestrator-%s", s.nodeName)

	var pid *goakt.PID
	var err error
	const maxRetries = 10
	for attempt := 1; attempt <= maxRetries; attempt++ {
		pid, err = s.actorSystem.Spawn(
			ctx,
			orchestratorName,
			actors.NewOrchestratorAgent(),
			goakt.WithLongLived(),
		)
		if err == nil {
			break
		}
		if attempt < maxRetries {
			delay := time.Duration(attempt) * 2 * time.Second
			s.logger.Warnf("spawn orchestrator attempt %d/%d failed: %v – retrying in %v",
				attempt, maxRetries, err, delay)

			time.Sleep(delay)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to spawn orchestrator after %d attempts: %w", maxRetries, err)
	}
	s.orchestratorPID = pid

	mux := http.NewServeMux()
	mux.HandleFunc("POST /query", s.handleQuery)
	mux.HandleFunc("GET /health", s.handleHealth)

	opts := []otelhttp.Option{}
	if s.tracerProvider != nil {
		opts = append(opts, otelhttp.WithTracerProvider(s.tracerProvider))
	}
	handler := otelhttp.NewHandler(mux, "goakt-ai", opts...)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           handler,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		s.logger.Infof("Query service listening on %s", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Errorf("query service error: %v", err)
		}
	}()

	return nil
}

// Stop stops the HTTP server
func (s *QueryService) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *QueryService) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	sessionID := uuid.New().String()
	reply, err := goakt.Ask(r.Context(), s.orchestratorPID, &messages.SubmitQuery{
		SessionID: sessionID,
		Query:     req.Query,
	}, askTimeout)
	if err != nil {
		s.logger.Errorf("query failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(QueryResponse{SessionID: sessionID, Error: err.Error()})
		return
	}

	resp, ok := reply.(*messages.QueryResponse)
	if !ok {
		http.Error(w, "invalid response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.Err != "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(QueryResponse{SessionID: sessionID, Error: resp.Err})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(QueryResponse{SessionID: sessionID, Result: resp.Result})
}

func (s *QueryService) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
