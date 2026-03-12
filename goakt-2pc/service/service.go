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
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	goakt "github.com/tochemey/goakt/v4/actor"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/tochemey/goakt-examples/v2/goakt-2pc/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/api"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/messages"
)

const askTimeout = 10 * time.Second

func spanNameFromRequest(_ string, r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}

// TransferService implements api.ServerInterface for the 2PC money transfer service.
type TransferService struct {
	actorSystem    goakt.ActorSystem
	logger         log.Logger
	port           int
	server         *http.Server
	tracerProvider trace.TracerProvider
}

var _ api.ServerInterface = (*TransferService)(nil)

func NewTransferService(system goakt.ActorSystem, port int, logger log.Logger, tracerProvider trace.TracerProvider) *TransferService {
	return &TransferService{
		actorSystem:    system,
		logger:         logger,
		port:           port,
		tracerProvider: tracerProvider,
	}
}

func (s *TransferService) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func()) {
	if s.tracerProvider == nil {
		return ctx, func() {}
	}
	tracer := s.tracerProvider.Tracer("2pc-transfer")
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, func() { span.End() }
}

func (s *TransferService) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req api.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	accountID := req.CreateAccount.AccountId
	balance := req.CreateAccount.AccountBalance
	
	s.logger.Info("Balance: %f", balance)

	ctx := r.Context()
	ctx, endSpawn := s.startSpan(ctx, "actor.Spawn", attribute.String("actor.id", accountID))
	accountEntity := actors.NewAccountEntity()
	pid, err := s.actorSystem.Spawn(ctx, accountID, accountEntity, goakt.WithLongLived())
	endSpawn()

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

func (s *TransferService) GetAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	ctx := r.Context()
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("actor.id", accountId))
	pid, err := s.actorSystem.ActorOf(ctx, accountId)
	endLookup()
	if err != nil {
		if errors.Is(err, gerrors.ErrActorNotFound) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		s.logger.Errorf("error locating actor: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if pid.IsLocal() {
		s.logger.Info("actor is found locally")
	}
	if pid.IsRemote() {
		s.logger.Infof("actor is found on remote node=%s", net.JoinHostPort(pid.Path().Host(), strconv.Itoa(pid.Path().Port())))
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountId))
	reply, err := goakt.Ask(ctx, pid, &messages.GetAccount{AccountID: accountId}, askTimeout)
	endAsk()
	if err != nil {
		s.logger.Errorf("error getting account: %v", err)
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

func (s *TransferService) CreditAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	var req api.CreditAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("actor.id", accountId))
	pid, err := s.actorSystem.ActorOf(ctx, accountId)
	endLookup()
	if err != nil {
		if errors.Is(err, gerrors.ErrActorNotFound) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		s.logger.Errorf("error locating actor: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountId))
	reply, err := goakt.Ask(ctx, pid, &messages.CreditAccount{
		AccountID: accountId,
		Amount:    req.Amount,
	}, askTimeout)
	endAsk()
	if err != nil {
		s.logger.Errorf("error crediting account: %v", err)
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

func (s *TransferService) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var req api.CreateTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	transferID := uuid.New().String()
	from := req.Transfer.FromAccountId
	to := req.Transfer.ToAccountId
	amount := req.Transfer.Amount

	if from == "" || to == "" || amount <= 0 {
		http.Error(w, "from_account_id, to_account_id and amount (positive) are required", http.StatusBadRequest)
		return
	}
	if from == to {
		http.Error(w, "from and to accounts must be different", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ctx, endSpawn := s.startSpan(ctx, "actor.Spawn", attribute.String("2pc.transfer_id", transferID))
	coordinator := actors.NewCoordinator()
	pid, err := s.actorSystem.Spawn(ctx, transferID, coordinator, goakt.WithLongLived())
	endSpawn()
	if err != nil {
		s.logger.Errorf("error spawning coordinator: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("2pc.transfer_id", transferID))
	reply, err := goakt.Ask(ctx, pid, &messages.StartTransfer{
		TransferID:    transferID,
		FromAccountID: from,
		ToAccountID:   to,
		Amount:        amount,
	}, askTimeout)
	endAsk()
	if err != nil {
		s.logger.Errorf("error executing transfer: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch resp := reply.(type) {
	case *messages.TransferCompleted:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(api.TransferResponse{
			Transfer: struct {
				TransferId string  `json:"transfer_id"`
				Status     string  `json:"status"`
				Reason     *string `json:"reason,omitempty"`
			}{
				TransferId: resp.TransferID,
				Status:     "completed",
			},
		})
	case *messages.TransferFailed:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		reason := resp.Reason
		_ = json.NewEncoder(w).Encode(api.TransferResponse{
			Transfer: struct {
				TransferId string  `json:"transfer_id"`
				Status     string  `json:"status"`
				Reason     *string `json:"reason,omitempty"`
			}{
				TransferId: resp.TransferID,
				Status:     "failed",
				Reason:     &reason,
			},
		})
	default:
		http.Error(w, fmt.Sprintf("invalid reply type: %T", reply), http.StatusInternalServerError)
	}
}

func (s *TransferService) GetTransfer(w http.ResponseWriter, r *http.Request, transferId string) {
	ctx := r.Context()
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("2pc.transfer_id", transferId))
	pid, err := s.actorSystem.ActorOf(ctx, transferId)
	endLookup()
	if err != nil {
		if errors.Is(err, gerrors.ErrActorNotFound) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			reason := "not_found"
			_ = json.NewEncoder(w).Encode(api.TransferStatusResponse{
				Transfer: struct {
					TransferId string  `json:"transfer_id"`
					Status     string  `json:"status"`
					Reason     *string `json:"reason,omitempty"`
				}{
					TransferId: transferId,
					Status:     "not_found",
					Reason:     &reason,
				},
			})
			return
		}
		s.logger.Errorf("error locating coordinator: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("2pc.transfer_id", transferId))
	reply, err := goakt.Ask(ctx, pid, &messages.GetTransferStatus{TransferID: transferId}, askTimeout)
	endAsk()
	if err != nil {
		s.logger.Errorf("error getting transfer status: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status, ok := reply.(*messages.TransferStatus)
	if !ok {
		http.Error(w, fmt.Sprintf("invalid reply type: %T", reply), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	var reason *string
	if status.Reason != "" {
		reason = &status.Reason
	}
	_ = json.NewEncoder(w).Encode(api.TransferStatusResponse{
		Transfer: struct {
			TransferId string  `json:"transfer_id"`
			Status     string  `json:"status"`
			Reason     *string `json:"reason,omitempty"`
		}{
			TransferId: status.TransferID,
			Status:     status.Status,
			Reason:     reason,
		},
	})
}

func (s *TransferService) Start() {
	go func() {
		s.listenAndServe()
	}()
}

func (s *TransferService) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *TransferService) listenAndServe() {
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
	wrappedHandler := otelhttp.NewHandler(handler, "2pc-transfer", opts...)
	serverAddr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:              serverAddr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       1200 * time.Second,
		Handler:           wrappedHandler,
	}

	s.logger.Infof("2PC transfer service listening on %s", serverAddr)
	if err := s.server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.logger.Errorf("failed to start service: %v", errors.Wrap(err, "listen error"))
		}
	}
}
