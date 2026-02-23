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
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/pkg/errors"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dynalloc/actors"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb/samplepbconnect"
)

const askTimeout = 5 * time.Second

type AccountService struct {
	actorSystem goakt.ActorSystem
	logger      log.Logger
	port        int
	server      *http.Server
}

var _ samplepbconnect.AccountServiceHandler = &AccountService{}

// NewAccountService creates an instance of AccountService
func NewAccountService(system goakt.ActorSystem, logger log.Logger, port int) *AccountService {
	return &AccountService{
		actorSystem: system,
		logger:      logger,
		port:        port,
	}
}

// CreateAccount helps create an account
func (s *AccountService) CreateAccount(ctx context.Context, c *connect.Request[samplepb.CreateAccountRequest]) (*connect.Response[samplepb.CreateAccountResponse], error) {
	// grab the actual request
	req := c.Msg
	// grab the account id
	accountID := req.GetCreateAccount().GetAccountId()
	// create the pid and send the command create account
	account := actors.NewAccount()

	s.logger.Infof("creating actor with id=%s", accountID)

	// create the given actor. This will spawn the actor on the cluster
	if err := s.actorSystem.SpawnOn(ctx, accountID, account, goakt.WithLongLived(), goakt.WithPlacement(goakt.Random)); err != nil {
		return nil, err
	}

	// Wait for the actor to properly start because it is done across cluster of nodes
	// TODO: Better way to do this is to create the actor first and in a second request credit his account
	time.Sleep(time.Second)

	s.logger.Infof("actor with id=%s is created", accountID)

	command := &samplepb.CreateAccount{
		AccountId:      accountID,
		AccountBalance: req.GetCreateAccount().GetAccountBalance(),
	}

	message, err := s.actorSystem.NoSender().SendSync(ctx, accountID, command, askTimeout)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	switch x := message.(type) {
	case *samplepb.Account:
		return connect.NewResponse(&samplepb.CreateAccountResponse{Account: x}), nil
	default:
		err := fmt.Errorf("invalid reply=%T", message)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
}

// CreditAccount helps credit a given account
func (s *AccountService) CreditAccount(ctx context.Context, c *connect.Request[samplepb.CreditAccountRequest]) (*connect.Response[samplepb.CreditAccountResponse], error) {
	req := c.Msg

	accountID := req.GetCreditAccount().GetAccountId()
	command := &samplepb.CreditAccount{
		AccountId: accountID,
		Balance:   req.GetCreditAccount().GetBalance(),
	}

	message, err := s.actorSystem.NoSender().SendSync(ctx, accountID, command, askTimeout)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	switch x := message.(type) {
	case *samplepb.Account:
		return connect.NewResponse(&samplepb.CreditAccountResponse{Account: x}), nil
	default:
		err := fmt.Errorf("invalid reply=%T", message)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
}

// GetAccount helps get an account
func (s *AccountService) GetAccount(ctx context.Context, c *connect.Request[samplepb.GetAccountRequest]) (*connect.Response[samplepb.GetAccountResponse], error) {
	// grab the actual request
	req := c.Msg
	accountID := req.GetAccountId()

	command := &samplepb.GetAccount{
		AccountId: accountID,
	}

	message, err := s.actorSystem.NoSender().SendSync(ctx, accountID, command, askTimeout)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// pattern match on the reply
	switch x := message.(type) {
	case *samplepb.Account:
		return connect.NewResponse(&samplepb.GetAccountResponse{Account: x}), nil
	default:
		err := fmt.Errorf("invalid reply=%T", message)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
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

// listenAndServe starts the http server
func (s *AccountService) listenAndServe() {
	// create a http service mux
	mux := http.NewServeMux()
	// create an interceptor
	interceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		s.logger.Panic(err)
	}
	// create the resource and handler
	path, handler := samplepbconnect.NewAccountServiceHandler(s,
		connect.WithInterceptors(interceptor))
	mux.Handle(path, handler)
	// create the address
	serverAddr := fmt.Sprintf(":%d", s.port)
	// create a http server instance
	server := &http.Server{
		Addr:              serverAddr,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      time.Second,
		IdleTimeout:       1200 * time.Second,
		Handler: h2c.NewHandler(mux, &http2.Server{
			IdleTimeout: 1200 * time.Second,
		}),
	}

	// set the server
	s.server = server
	// listen and service requests
	if err := s.server.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return
		}
		s.logger.Panic(errors.Wrap(err, "failed to start actor-remoting service"))
	}
}
