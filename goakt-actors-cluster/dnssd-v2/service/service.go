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

	"github.com/pkg/errors"
	goakt "github.com/tochemey/goakt/v4/actor"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/api"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/messages"
)

const askTimeout = 5 * time.Second

// AccountService implements api.ServerInterface and backs it with the actor system.
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
	accountEntity := actors.NewAccountEntity()
	pid, err := s.actorSystem.Spawn(ctx, accountID, accountEntity, goakt.WithLongLived())
	if err != nil {
		s.logger.Errorf("error spawning actor: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reply, err := goakt.Ask(ctx, pid, &messages.CreateAccount{
		AccountID:      accountID,
		AccountBalance: balance,
	}, time.Second)
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

	ctx := r.Context()
	pid, err := s.actorSystem.ActorOf(ctx, accountId)
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

	reply, err := goakt.Ask(ctx, pid, &messages.CreditAccount{
		AccountID: accountId,
		Balance:   req.Balance,
	}, time.Second)
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

// GetAccount implements api.ServerInterface.
func (s *AccountService) GetAccount(w http.ResponseWriter, r *http.Request, accountId string) {
	ctx := r.Context()
	pid, err := s.actorSystem.ActorOf(ctx, accountId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if pid.IsLocal() {
		s.logger.Info("actor is found locally")
	}
	if pid.IsRemote() {
		s.logger.Infof("actor is found on remote node=%s", net.JoinHostPort(pid.Path().Host(), strconv.Itoa(pid.Path().Port())))
	}

	reply, err := goakt.Ask(ctx, pid, &messages.GetAccount{AccountID: accountId}, askTimeout)
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
	serverAddr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:              serverAddr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       1200 * time.Second,
		Handler:           handler,
	}

	s.logger.Infof("Account service listening on %s", serverAddr)
	if err := s.server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.logger.Errorf("failed to start actor-remoting service: %v", errors.Wrap(err, "listen error"))
		}
	}
}
