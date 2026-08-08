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
	"errors"

	"connectrpc.com/connect"
	gerrors "github.com/tochemey/goakt/v4/errors"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/dnssd/messages"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb/samplepbconnect"
)

// rpcService is the gRPC-compatible Connect RPC face of AccountService, generated
// from protos/sample/service.proto.
//
// It owns no state and makes no decisions: every method translates a protobuf
// request into the plain Go structs the actors speak, delegates to the shared
// AccountService calls, and translates the reply back. That keeps the REST and
// RPC surfaces behaviourally identical — they run the same code underneath.
type rpcService struct {
	service *AccountService
}

var _ samplepbconnect.AccountServiceHandler = (*rpcService)(nil)

// CreateAccount implements samplepbconnect.AccountServiceHandler.
func (r *rpcService) CreateAccount(ctx context.Context, c *connect.Request[samplepb.CreateAccountRequest]) (*connect.Response[samplepb.CreateAccountResponse], error) {
	account, err := r.service.create(ctx,
		c.Msg.GetCreateAccount().GetAccountId(),
		c.Msg.GetCreateAccount().GetAccountBalance())
	if err != nil {
		r.service.logger.Errorf("error creating account: %v", err)
		return nil, connectError(err)
	}

	return connect.NewResponse(&samplepb.CreateAccountResponse{Account: toProtoAccount(account)}), nil
}

// CreditAccount implements samplepbconnect.AccountServiceHandler.
func (r *rpcService) CreditAccount(ctx context.Context, c *connect.Request[samplepb.CreditAccountRequest]) (*connect.Response[samplepb.CreditAccountResponse], error) {
	account, err := r.service.credit(ctx,
		c.Msg.GetCreditAccount().GetAccountId(),
		c.Msg.GetCreditAccount().GetBalance())
	if err != nil {
		r.service.logger.Errorf("error crediting account: %v", err)
		return nil, connectError(err)
	}

	return connect.NewResponse(&samplepb.CreditAccountResponse{Account: toProtoAccount(account)}), nil
}

// GetAccount implements samplepbconnect.AccountServiceHandler.
func (r *rpcService) GetAccount(ctx context.Context, c *connect.Request[samplepb.GetAccountRequest]) (*connect.Response[samplepb.GetAccountResponse], error) {
	account, err := r.service.get(ctx, c.Msg.GetAccountId())
	if err != nil {
		r.service.logger.Errorf("error getting account: %v", err)
		return nil, connectError(err)
	}

	return connect.NewResponse(&samplepb.GetAccountResponse{Account: toProtoAccount(account)}), nil
}

// toProtoAccount converts the actors' account struct into its protobuf form.
func toProtoAccount(account *messages.Account) *samplepb.Account {
	return &samplepb.Account{
		AccountId:      account.AccountID,
		AccountBalance: account.AccountBalance,
	}
}

// connectError maps an actor-system error onto a Connect status code, mirroring
// how the REST handlers map the same errors onto HTTP status codes.
func connectError(err error) error {
	if isNotFound(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

// isNotFound reports whether err means the account entity does not exist.
func isNotFound(err error) bool {
	return errors.Is(err, gerrors.ErrActorNotFound)
}
