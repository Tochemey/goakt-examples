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
	"connectrpc.com/otelconnect"
	"github.com/pkg/errors"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/dnssd/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/dnssd/api"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/dnssd/messages"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb/samplepbconnect"
)

const askTimeout = 5 * time.Second

// AccountService exposes the account actors over two client-facing APIs at once:
//
//   - a REST/JSON API generated from api/openapi.yaml, which it implements
//     directly as api.ServerInterface
//   - a gRPC-compatible Connect RPC API generated from protos/sample/service.proto,
//     implemented by rpcService in rpc.go
//
// Both surfaces are served from the same port and both funnel into the same
// actor calls below, so neither API can drift from the other. The actors
// themselves only ever see the Go structs in the messages package; translating
// to and from protobuf is the RPC layer's job.
type AccountService struct {
	actorSystem goakt.ActorSystem
	logger      log.Logger
	port        int
	server      *http.Server
}

var _ api.ServerInterface = (*AccountService)(nil)

// NewAccountService creates an instance of AccountService
func NewAccountService(system goakt.ActorSystem, port int, logger log.Logger) *AccountService {
	return &AccountService{
		actorSystem: system,
		logger:      logger,
		port:        port,
	}
}

// create spawns the account entity and applies the create command.
func (s *AccountService) create(ctx context.Context, accountID string, balance float64) (*messages.Account, error) {
	pid, err := s.actorSystem.Spawn(ctx, accountID, actors.NewAccountEntity(), goakt.WithLongLived())
	if err != nil {
		return nil, err
	}

	reply, err := goakt.Ask(ctx, pid, &messages.CreateAccount{
		AccountID:      accountID,
		AccountBalance: balance,
	}, time.Second)
	if err != nil {
		return nil, err
	}
	return toAccount(reply)
}

// credit locates the account entity and credits it.
func (s *AccountService) credit(ctx context.Context, accountID string, balance float64) (*messages.Account, error) {
	pid, err := s.actorSystem.ActorOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	s.logLocation(pid)

	reply, err := goakt.Ask(ctx, pid, &messages.CreditAccount{
		AccountID: accountID,
		Balance:   balance,
	}, time.Second)
	if err != nil {
		return nil, err
	}
	return toAccount(reply)
}

// get locates the account entity and reads its balance.
func (s *AccountService) get(ctx context.Context, accountID string) (*messages.Account, error) {
	pid, err := s.actorSystem.ActorOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	s.logLocation(pid)

	reply, err := goakt.Ask(ctx, pid, &messages.GetAccount{AccountID: accountID}, askTimeout)
	if err != nil {
		return nil, err
	}
	return toAccount(reply)
}

// logLocation reports whether the entity was found on this node or another one.
func (s *AccountService) logLocation(pid *goakt.PID) {
	if pid.IsLocal() {
		s.logger.Info("actor is found locally")
	}
	if pid.IsRemote() {
		s.logger.Infof("actor is found on remote node=%s", net.JoinHostPort(pid.Path().Host(), strconv.Itoa(pid.Path().Port())))
	}
}

// toAccount narrows an actor reply to an account.
func toAccount(reply any) (*messages.Account, error) {
	account, ok := reply.(*messages.Account)
	if !ok {
		return nil, fmt.Errorf("invalid reply type: %T", reply)
	}
	return account, nil
}

// CreateAccount implements api.ServerInterface.
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

// CreditAccount implements api.ServerInterface.
func (s *AccountService) CreditAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	var req api.CreditAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	account, err := s.credit(r.Context(), accountId, req.Balance)
	if err != nil {
		if isNotFound(err) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		s.logger.Errorf("error crediting account: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeAccount(w, account)
}

// GetAccount implements api.ServerInterface.
func (s *AccountService) GetAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	account, err := s.get(r.Context(), accountId)
	if err != nil {
		s.logger.Errorf("error getting account: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeAccount(w, account)
}

// writeAccount renders an account as the JSON response body.
func writeAccount(w http.ResponseWriter, account *messages.Account) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(api.AccountResponse{Account: api.Account{
		AccountId:      account.AccountID,
		AccountBalance: account.AccountBalance,
	}})
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

	// Connect RPC surface. The handler is mounted on its own generated path
	// (/samplepb.AccountService/), so it cannot collide with the REST routes.
	interceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		s.logger.Errorf("failed to create the otel interceptor: %v", err)
		return
	}
	rpcPath, rpcHandler := samplepbconnect.NewAccountServiceHandler(
		&rpcService{service: s},
		connect.WithInterceptors(interceptor),
	)
	mux.Handle(rpcPath, rpcHandler)

	// REST surface, layered on the same mux.
	handler := api.HandlerWithOptions(s, api.StdHTTPServerOptions{BaseRouter: mux})

	// gRPC clients need HTTP/2, and without TLS that means h2c. Enabling both
	// protocols on the server lets HTTP/1.1 REST traffic and h2c gRPC traffic
	// share one port. (This replaces the deprecated golang.org/x/net/http2/h2c
	// wrapper the earlier version of this example used.)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	serverAddr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:              serverAddr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       1200 * time.Second,
		Protocols:         protocols,
		Handler:           handler,
	}

	s.logger.Infof("Account service listening on %s (REST + Connect RPC at %s)", serverAddr, rpcPath)
	if err := s.server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.logger.Errorf("failed to start actor-remoting service: %v", errors.Wrap(err, "listen error"))
		}
	}
}
