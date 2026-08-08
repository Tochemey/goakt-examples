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

package actors

import (
	"reflect"
	"time"

	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/domain"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/persistence"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/wire"
)

var zeroTime = time.Time{}

// AccountEntity represents the actor implementation using Go structs with persistence.
// It speaks only the domain model in messages; [wire.Decode] / [wire.Codec.Encode]
// translate to and from the process-wide remoting codec.
type AccountEntity struct {
	state   *domain.Account
	storage persistence.Store
}

var _ actor.Actor = (*AccountEntity)(nil)

// NewAccountEntity creates an instance of AccountEntity
func NewAccountEntity() *AccountEntity {
	return &AccountEntity{}
}

// PreStart is used to pre-set initial values for the actor
func (x *AccountEntity) PreStart(ctx *actor.Context) error {
	accountID := ctx.ActorName()
	x.storage = ctx.Extension(persistence.PostgresStateStoreID).(persistence.Store)
	latestState, err := x.storage.GetState(ctx.Context(), accountID)
	if err != nil {
		return err
	}
	x.state = domain.NewAccount(accountID, 0, zeroTime)
	x.state = latestState
	return nil
}

// Receive handles the messages sent to the actor
func (x *AccountEntity) Receive(ctx *actor.ReceiveContext) {
	message, codec := wire.Decode(ctx.Message())
	switch msg := message.(type) {
	case *actor.PostStart:
		state := x.state
		if state != nil && reflect.DeepEqual(state, new(domain.Account)) {
			state.SetCreatedAt(zeroTime)
			state.SetBalance(0)
		}

	case *messages.CreateAccount:
		ctx.Logger().Info("creating account by setting the balance...")
		state := x.state

		// check whether the create operation has been done already
		if !state.CreatedAt().Equal(zeroTime) {
			ctx.Logger().Infof("account=%s has been created already", state.AccountID())
			x.respond(ctx, codec, &messages.Account{
				AccountID:      state.AccountID(),
				AccountBalance: state.Balance(),
			})
			return
		}

		accountID := msg.AccountID
		balance := msg.AccountBalance

		state.SetBalance(balance)
		state.SetCreatedAt(time.Now())
		x.state = state

		x.respond(ctx, codec, &messages.Account{
			AccountID:      accountID,
			AccountBalance: state.Balance(),
		})

	case *messages.CreditAccount:
		ctx.Logger().Info("crediting balance...")
		state := x.state

		accountID := msg.AccountID
		balance := msg.Balance

		state.SetBalance(state.Balance() + balance)
		x.state = state
		x.respond(ctx, codec, &messages.Account{
			AccountID:      accountID,
			AccountBalance: state.Balance(),
		})

	case *messages.GetAccount:
		ctx.Logger().Info("get account...")
		state := x.state
		x.respond(ctx, codec, &messages.Account{
			AccountID:      msg.AccountID,
			AccountBalance: state.Balance(),
		})
	default:
		ctx.Unhandled()
	}
}

func (x *AccountEntity) respond(ctx *actor.ReceiveContext, codec wire.Codec, account *messages.Account) {
	if codec == nil {
		ctx.Response(account)
		return
	}
	ctx.Response(codec.Encode(account))
}

// PostStop is used to free-up resources when the actor stops
func (x *AccountEntity) PostStop(ctx *actor.Context) error {
	underlying := x.state
	if underlying == nil {
		return nil
	}
	return x.storage.WriteState(ctx.Context(), underlying.AccountID(), underlying)
}
