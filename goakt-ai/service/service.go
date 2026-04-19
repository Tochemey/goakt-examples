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
	Query     string `json:"query"`
	SessionID string `json:"session_id,omitempty"`
}

// QueryResponse is the HTTP response for POST /query
type QueryResponse struct {
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

// QueryService serves the query HTTP endpoints. Each incoming request is
// routed to a ConversationGrain keyed by SessionID so a session's turns are
// serialized by a single-writer grain regardless of which node received the
// HTTP call — the grain is placed by the cluster, not the HTTP entry point.
type QueryService struct {
	actorSystem    goakt.ActorSystem
	logger         log.Logger
	port           int
	nodeName       string
	server         *http.Server
	tracerProvider trace.TracerProvider
}

// NewQueryService creates a new query service.
// nodeName must be unique per cluster node (e.g. the pod hostname).
func NewQueryService(actorSystem goakt.ActorSystem, port int, nodeName string, logger log.Logger, tracerProvider trace.TracerProvider) *QueryService {
	return &QueryService{
		actorSystem:    actorSystem,
		logger:         logger,
		port:           port,
		nodeName:       nodeName,
		tracerProvider: tracerProvider,
	}
}

// Start begins serving HTTP. No orchestrator actor is spawned here; the
// grain is resolved lazily per-request via ActorSystem.GrainIdentity, which
// activates the grain on its cluster-assigned node when needed.
func (queryService *QueryService) Start(_ context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /query", queryService.handleQuery)
	mux.HandleFunc("GET /query/stream", queryService.handleStreamQuery)
	mux.HandleFunc("GET /health", queryService.handleHealth)

	tracingOptions := []otelhttp.Option{}
	if queryService.tracerProvider != nil {
		tracingOptions = append(tracingOptions, otelhttp.WithTracerProvider(queryService.tracerProvider))
	}

	handler := otelhttp.NewHandler(mux, "goakt-ai", tracingOptions...)

	queryService.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", queryService.port),
		Handler:           handler,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		queryService.logger.Infof("Query service listening on %s", queryService.server.Addr)
		if err := queryService.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			queryService.logger.Errorf("query service error: %v", err)
		}
	}()

	return nil
}

// Stop stops the HTTP server
func (queryService *QueryService) Stop(ctx context.Context) error {
	return queryService.server.Shutdown(ctx)
}

func (queryService *QueryService) handleQuery(responseWriter http.ResponseWriter, request *http.Request) {
	var queryRequest QueryRequest
	if err := json.NewDecoder(request.Body).Decode(&queryRequest); err != nil {
		http.Error(responseWriter, "invalid request body", http.StatusBadRequest)
		return
	}

	if queryRequest.Query == "" {
		http.Error(responseWriter, "query is required", http.StatusBadRequest)
		return
	}

	sessionID := queryRequest.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	grainIdentity, err := queryService.actorSystem.GrainIdentity(
		request.Context(),
		sessionID,
		actors.NewConversationGrainFactory(),
		goakt.WithGrainDeactivateAfter(10*time.Minute),
	)
	if err != nil {
		queryService.writeErr(responseWriter, sessionID, http.StatusInternalServerError, fmt.Errorf("grain identity: %w", err))
		return
	}

	reply, err := queryService.actorSystem.AskGrain(request.Context(), grainIdentity, &messages.SubmitQuery{
		SessionID: sessionID,
		Query:     queryRequest.Query,
	}, askTimeout)
	if err != nil {
		queryService.writeErr(responseWriter, sessionID, http.StatusInternalServerError, err)
		return
	}

	queryResponse, ok := reply.(*messages.QueryResponse)
	if !ok {
		http.Error(responseWriter, "invalid response", http.StatusInternalServerError)
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	if queryResponse.Err != "" {
		responseWriter.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(responseWriter).Encode(QueryResponse{SessionID: sessionID, Error: queryResponse.Err})
		return
	}

	responseWriter.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(responseWriter).Encode(QueryResponse{SessionID: sessionID, Result: queryResponse.Result})
}

func (queryService *QueryService) writeErr(responseWriter http.ResponseWriter, sessionID string, status int, err error) {
	queryService.logger.Errorf("query failed: %v", err)
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(status)
	_ = json.NewEncoder(responseWriter).Encode(QueryResponse{SessionID: sessionID, Error: err.Error()})
}

func (queryService *QueryService) handleHealth(responseWriter http.ResponseWriter, _ *http.Request) {
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write([]byte("ok"))
}
