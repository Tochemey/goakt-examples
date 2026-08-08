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

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	goakt "github.com/tochemey/goakt/v4/actor"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/api"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/wire"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb/samplepbconnect"
)

const askTimeout = 5 * time.Second

// spanNameFromRequest returns a clean span name for HTTP requests.
func spanNameFromRequest(_ string, r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}

// AccountService backs the account API with the actor system.
//
// Exactly one client façade is mounted, selected by codec:
//   - cbor → OpenAPI HTTP/JSON
//   - proto → Connect/gRPC
type AccountService struct {
	actorSystem    goakt.ActorSystem
	logger         log.Logger
	port           int
	server         *http.Server
	tracerProvider trace.TracerProvider
	codec          wire.Codec
}

var _ api.ServerInterface = (*AccountService)(nil)

// NewAccountService creates an instance of AccountService.
// tracerProvider is used for HTTP and actor span instrumentation; pass nil to disable.
func NewAccountService(system goakt.ActorSystem, port int, logger log.Logger, tracerProvider trace.TracerProvider, codec wire.Codec) *AccountService {
	return &AccountService{
		actorSystem:    system,
		logger:         logger,
		port:           port,
		tracerProvider: tracerProvider,
		codec:          codec,
	}
}

func (s *AccountService) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func()) {
	if s.tracerProvider == nil {
		return ctx, func() {}
	}
	tracer := s.tracerProvider.Tracer("accounts")
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, func() { span.End() }
}

func (s *AccountService) create(ctx context.Context, accountID string, balance float64) (*messages.Account, error) {
	ctx, endSpawn := s.startSpan(ctx, "actor.Spawn", attribute.String("actor.id", accountID))
	pid, err := s.actorSystem.Spawn(ctx, accountID, actors.NewAccountEntity(), goakt.WithLongLived())
	endSpawn()
	if err != nil {
		return nil, err
	}

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountID))
	reply, err := goakt.Ask(ctx, pid, s.codec.Encode(&messages.CreateAccount{
		AccountID:      accountID,
		AccountBalance: balance,
	}), time.Second)
	endAsk()
	if err != nil {
		return nil, err
	}
	return toAccount(reply)
}

func (s *AccountService) credit(ctx context.Context, accountID string, balance float64) (*messages.Account, error) {
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("actor.id", accountID))
	pid, err := s.actorSystem.ActorOf(ctx, accountID)
	endLookup()
	if err != nil {
		return nil, err
	}
	s.logPlacement(pid)

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountID))
	reply, err := goakt.Ask(ctx, pid, s.codec.Encode(&messages.CreditAccount{
		AccountID: accountID,
		Balance:   balance,
	}), time.Second)
	endAsk()
	if err != nil {
		return nil, err
	}
	return toAccount(reply)
}

func (s *AccountService) get(ctx context.Context, accountID string) (*messages.Account, error) {
	ctx, endLookup := s.startSpan(ctx, "actor.ActorOf", attribute.String("actor.id", accountID))
	pid, err := s.actorSystem.ActorOf(ctx, accountID)
	endLookup()
	if err != nil {
		return nil, err
	}
	s.logPlacement(pid)

	ctx, endAsk := s.startSpan(ctx, "actor.Ask", attribute.String("actor.id", accountID))
	reply, err := goakt.Ask(ctx, pid, s.codec.Encode(&messages.GetAccount{AccountID: accountID}), askTimeout)
	endAsk()
	if err != nil {
		return nil, err
	}
	return toAccount(reply)
}

func (s *AccountService) logPlacement(pid *goakt.PID) {
	if pid.IsLocal() {
		s.logger.Info("actor is found locally")
	}
	if pid.IsRemote() {
		s.logger.Infof("actor is found on remote node=%s", net.JoinHostPort(pid.Path().Host(), strconv.Itoa(pid.Path().Port())))
	}
}

func toAccount(reply any) (*messages.Account, error) {
	decoded, _ := wire.Decode(reply)
	acc, ok := decoded.(*messages.Account)
	if !ok {
		return nil, fmt.Errorf("invalid reply type: %T", reply)
	}
	return acc, nil
}

// CreateAccount implements api.ServerInterface (cbor / HTTP mode).
func (s *AccountService) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req api.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	account, err := s.create(r.Context(), req.CreateAccount.AccountId, req.CreateAccount.AccountBalance)
	if err != nil {
		s.logger.Errorf("error creating account: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeAccount(w, account)
}

// CreditAccount implements api.ServerInterface (cbor / HTTP mode).
func (s *AccountService) CreditAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	var req api.CreditAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	account, err := s.credit(r.Context(), accountId, req.Balance)
	if err != nil {
		if errors.Is(err, gerrors.ErrActorNotFound) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		s.logger.Errorf("error crediting account: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeAccount(w, account)
}

// GetAccount implements api.ServerInterface (cbor / HTTP mode).
func (s *AccountService) GetAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	account, err := s.get(r.Context(), accountId)
	if err != nil {
		s.logger.Errorf("error getting account: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeAccount(w, account)
}

func writeAccount(w http.ResponseWriter, account *messages.Account) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.AccountResponse{Account: api.Account{
		AccountId:      account.AccountID,
		AccountBalance: account.AccountBalance,
	}})
}

// rpcService is the Connect/gRPC face used only in proto mode.
type rpcService struct {
	service *AccountService
}

var _ samplepbconnect.AccountServiceHandler = (*rpcService)(nil)

func (r *rpcService) CreateAccount(ctx context.Context, c *connect.Request[samplepb.CreateAccountRequest]) (*connect.Response[samplepb.CreateAccountResponse], error) {
	account, err := r.service.create(ctx,
		c.Msg.GetCreateAccount().GetAccountId(),
		c.Msg.GetCreateAccount().GetAccountBalance())
	if err != nil {
		r.service.logger.Errorf("error creating account: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&samplepb.CreateAccountResponse{Account: toProtoAccount(account)}), nil
}

func (r *rpcService) CreditAccount(ctx context.Context, c *connect.Request[samplepb.CreditAccountRequest]) (*connect.Response[samplepb.CreditAccountResponse], error) {
	account, err := r.service.credit(ctx,
		c.Msg.GetCreditAccount().GetAccountId(),
		c.Msg.GetCreditAccount().GetBalance())
	if err != nil {
		r.service.logger.Errorf("error crediting account: %v", err)
		if errors.Is(err, gerrors.ErrActorNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&samplepb.CreditAccountResponse{Account: toProtoAccount(account)}), nil
}

func (r *rpcService) GetAccount(ctx context.Context, c *connect.Request[samplepb.GetAccountRequest]) (*connect.Response[samplepb.GetAccountResponse], error) {
	account, err := r.service.get(ctx, c.Msg.GetAccountId())
	if err != nil {
		r.service.logger.Errorf("error getting account: %v", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&samplepb.GetAccountResponse{Account: toProtoAccount(account)}), nil
}

func toProtoAccount(account *messages.Account) *samplepb.Account {
	encoded := wire.ProtoCodec.Encode(account)
	return encoded.(*samplepb.Account)
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
	if s.codec.Name() == wire.Proto {
		s.listenProto()
		return
	}
	s.listenHTTP()
}

func (s *AccountService) listenHTTP() {
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

	s.logger.Infof("Account service listening on %s (codec=%s, HTTP/JSON)", serverAddr, s.codec.Name())
	if err := s.server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.logger.Errorf("failed to start account service: %v", errors.Wrap(err, "listen error"))
		}
	}
}

func (s *AccountService) listenProto() {
	mux := http.NewServeMux()
	path, handler := samplepbconnect.NewAccountServiceHandler(&rpcService{service: s})
	mux.Handle(path, handler)

	serverAddr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:              serverAddr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       1200 * time.Second,
		Handler: h2c.NewHandler(mux, &http2.Server{
			IdleTimeout: 1200 * time.Second,
		}),
	}

	s.logger.Infof("Account service listening on %s (codec=%s, Connect/gRPC)", serverAddr, s.codec.Name())
	if err := s.server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.logger.Errorf("failed to start account service: %v", errors.Wrap(err, "listen error"))
		}
	}
}
