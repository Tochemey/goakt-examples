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

	"github.com/pkg/errors"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/datacenter"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/api"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/messages"
)

const askTimeout = 5 * time.Second

// spanNameFromRequest returns a clean span name for HTTP requests.
func spanNameFromRequest(_ string, r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}

// AccountService implements api.ServerInterface and backs it with the actor system.
type AccountService struct {
	actorSystem    goakt.ActorSystem
	logger         log.Logger
	port           int
	dcName         string
	server         *http.Server
	tracerProvider trace.TracerProvider
}

var _ api.ServerInterface = (*AccountService)(nil)

// NewAccountService creates an instance of AccountService.
func NewAccountService(system goakt.ActorSystem, port int, dcName string, logger log.Logger, tracerProvider trace.TracerProvider) *AccountService {
	return &AccountService{
		actorSystem:    system,
		logger:         logger,
		port:           port,
		dcName:         dcName,
		tracerProvider: tracerProvider,
	}
}

// forwardViaGateway delegates a cross-DC actor lookup to the dc-gateway singleton.
// The gateway runs on the cluster leader where the DC controller is available,
// enabling SendSync to use DiscoverActor for cross-datacenter resolution.
func (s *AccountService) forwardViaGateway(ctx context.Context, msg any) (any, error) {
	gatewayPID, err := s.actorSystem.ActorOf(ctx, "dc-gateway")
	if err != nil {
		return nil, fmt.Errorf("dc-gateway not available: %w", err)
	}
	return goakt.Ask(ctx, gatewayPID, msg, askTimeout)
}

// startSpan starts a child span when tracing is enabled. Returns ctx and a no-op end if disabled.
func (s *AccountService) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func()) {
	if s.tracerProvider == nil {
		return ctx, func() {}
	}
	tracer := s.tracerProvider.Tracer("accounts")
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, func() { span.End() }
}

// CreateAccount implements api.ServerInterface.
func (s *AccountService) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req api.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	accountID := req.CreateAccount.AccountId
	balance := req.CreateAccount.AccountBalance

	ctx := r.Context()
	ctx, endSpawn := s.startSpan(ctx, "actor.Spawn", attribute.String("actor.id", accountID))
	accountEntity := actors.NewAccountEntity()
	pid, err := s.actorSystem.Spawn(ctx, accountID, accountEntity, goakt.WithLongLived())
	endSpawn()
	if err != nil {
		s.logger.Errorf("error spawning actor: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountID))
	reply, err := goakt.Ask(ctx, pid, &messages.CreateAccount{
		AccountID:      accountID,
		AccountBalance: balance,
	}, time.Second)
	endAsk()
	if err != nil {
		s.logger.Errorf("error creating account: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	acc, ok := reply.(*messages.Account)
	if !ok {
		http.Error(w, fmt.Sprintf("invalid reply type: %T", reply), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.AccountResponse{Account: api.Account{
		AccountId:      acc.AccountID,
		AccountBalance: acc.AccountBalance,
	}})
}

// CreditAccount implements api.ServerInterface.
func (s *AccountService) CreditAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	var req api.CreditAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	msg := &messages.CreditAccount{
		AccountID: accountId,
		Balance:   req.Balance,
	}

	ctx := r.Context()

	// Try local cluster first via ActorOf
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("actor.id", accountId))
	pid, err := s.actorSystem.ActorOf(ctx, accountId)
	endLookup()
	if err == nil {
		ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountId))
		reply, askErr := goakt.Ask(ctx, pid, msg, askTimeout)
		endAsk()
		if askErr != nil {
			s.logger.Errorf("error crediting account: %v", askErr)
			http.Error(w, askErr.Error(), http.StatusInternalServerError)
			return
		}
		s.writeAccountResponse(w, reply)
		return
	}

	if !errors.Is(err, gerrors.ErrActorNotFound) {
		s.logger.Errorf("error locating actor: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cross-DC lookup via the dc-gateway singleton on the leader node
	ctx, endForward := s.startSpan(ctx, "actor.ForwardViaGateway", attribute.String("actor.id", accountId))
	reply, err := s.forwardViaGateway(ctx, &messages.ForwardCreditAccount{AccountID: accountId, Balance: req.Balance})
	endForward()
	if err != nil {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	acc, ok := reply.(*messages.Account)
	if !ok || acc.AccountID == "" {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	s.writeAccountResponse(w, reply)
}

// GetAccount implements api.ServerInterface.
func (s *AccountService) GetAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	ctx := r.Context()

	// Try local cluster first via ActorOf
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("actor.id", accountId))
	pid, err := s.actorSystem.ActorOf(ctx, accountId)
	endLookup()
	if err == nil {
		ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountId))
		reply, askErr := goakt.Ask(ctx, pid, &messages.GetAccount{AccountID: accountId}, askTimeout)
		endAsk()
		if askErr != nil {
			s.logger.Errorf("error getting account: %v", askErr)
			http.Error(w, askErr.Error(), http.StatusInternalServerError)
			return
		}
		s.writeAccountResponse(w, reply)
		return
	}

	if !errors.Is(err, gerrors.ErrActorNotFound) {
		s.logger.Errorf("error locating actor: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cross-DC lookup via the dc-gateway singleton on the leader node
	ctx, endForward := s.startSpan(ctx, "actor.ForwardViaGateway", attribute.String("actor.id", accountId))
	reply, err := s.forwardViaGateway(ctx, &messages.ForwardGetAccount{AccountID: accountId})
	endForward()
	if err != nil {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	acc, ok := reply.(*messages.Account)
	if !ok || acc.AccountID == "" {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}
	s.writeAccountResponse(w, reply)
}

// writeAccountResponse encodes an *messages.Account reply as JSON.
func (s *AccountService) writeAccountResponse(w http.ResponseWriter, reply any) {
	acc, ok := reply.(*messages.Account)
	if !ok {
		http.Error(w, fmt.Sprintf("invalid reply type: %T", reply), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.AccountResponse{Account: api.Account{
		AccountId:      acc.AccountID,
		AccountBalance: acc.AccountBalance,
	}})
}

// SpawnRemoteAccount implements api.ServerInterface.
// Spawns an account actor in a remote datacenter using SpawnOn with WithDataCenter.
func (s *AccountService) SpawnRemoteAccount(w http.ResponseWriter, r *http.Request) {
	var req api.SpawnRemoteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	targetDC := &datacenter.DataCenter{Name: req.TargetDc}

	ctx, endSpawn := s.startSpan(ctx, "actor.SpawnOn",
		attribute.String("actor.id", req.AccountId),
		attribute.String("target.dc", req.TargetDc))

	accountEntity := actors.NewAccountEntity()
	pid, err := s.actorSystem.SpawnOn(ctx, req.AccountId, accountEntity,
		goakt.WithDataCenter(targetDC),
		goakt.WithLongLived())
	endSpawn()
	if err != nil {
		s.logger.Errorf("error spawning actor on remote DC %s: %v", req.TargetDc, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", req.AccountId))
	reply, err := goakt.Ask(ctx, pid, &messages.CreateAccount{
		AccountID:      req.AccountId,
		AccountBalance: req.AccountBalance,
	}, askTimeout)
	endAsk()
	if err != nil {
		s.logger.Errorf("error creating account on remote DC: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	acc, ok := reply.(*messages.Account)
	if !ok {
		http.Error(w, fmt.Sprintf("invalid reply type: %T", reply), http.StatusInternalServerError)
		return
	}

	s.logger.Infof("account %s spawned on remote DC %s", req.AccountId, req.TargetDc)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.AccountResponse{Account: api.Account{
		AccountId:      acc.AccountID,
		AccountBalance: acc.AccountBalance,
	}})
}

// GetDCStatus implements api.ServerInterface.
// Returns datacenter readiness and last refresh time.
func (s *AccountService) GetDCStatus(w http.ResponseWriter, r *http.Request) {
	ready := s.actorSystem.DataCenterReady()
	lastRefresh := s.actorSystem.DataCenterLastRefresh()

	resp := api.DCStatusResponse{
		Ready:  ready,
		DcName: s.dcName,
	}

	if !lastRefresh.IsZero() {
		resp.LastRefresh = &lastRefresh
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// Start starts the service
func (s *AccountService) Start() {
	go func() {
		s.listenAndServe()
	}()
}

// Stop stops the service
func (s *AccountService) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *AccountService) listenAndServe() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPI)
	mux.HandleFunc("GET /docs", serveSwaggerUI)
	mux.HandleFunc("GET /swagger", serveSwaggerUI)

	handler := api.HandlerWithOptions(s, api.StdHTTPServerOptions{BaseRouter: mux})
	opts := []otelhttp.Option{
		otelhttp.WithSpanNameFormatter(spanNameFromRequest),
	}
	if s.tracerProvider != nil {
		opts = append(opts, otelhttp.WithTracerProvider(s.tracerProvider))
	}
	wrappedHandler := otelhttp.NewHandler(handler, "accounts", opts...)
	serverAddr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:              serverAddr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       1200 * time.Second,
		Handler:           wrappedHandler,
	}

	s.logger.Infof("Account service listening on %s", serverAddr)
	if err := s.server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.logger.Errorf("failed to start actor-remoting service: %v", errors.Wrap(err, "listen error"))
		}
	}
}
