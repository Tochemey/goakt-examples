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

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/domain"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/persistence"
)

const forwardTimeout = 5 * time.Second

var zeroTime = time.Time{}

// DataCenterGateway is a cluster singleton actor that handles cross-DC actor lookups.
// It runs on the leader node where the DC controller is available, enabling
// SendSync to use DiscoverActor for cross-datacenter resolution.
type DataCenterGateway struct{}

var _ actor.Actor = (*DataCenterGateway)(nil)

func (g *DataCenterGateway) PreStart(*actor.Context) error { return nil }
func (g *DataCenterGateway) PostStop(*actor.Context) error { return nil }

func (g *DataCenterGateway) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ForwardGetAccount:
		g.forwardAndReply(ctx, msg.AccountID, &messages.GetAccount{AccountID: msg.AccountID})
	case *messages.ForwardCreditAccount:
		g.forwardAndReply(ctx, msg.AccountID, &messages.CreditAccount{AccountID: msg.AccountID, Balance: msg.Balance})
	default:
		ctx.Unhandled()
	}
}

// forwardAndReply uses Self().SendSync to discover and message an actor across DCs.
// It avoids ctx.SendSync to prevent supervisor suspension on lookup failures.
// On failure, responds with an empty Account to signal "not found" over remoting.
func (g *DataCenterGateway) forwardAndReply(ctx *actor.ReceiveContext, actorName string, msg any) {
	reply, err := ctx.Self().SendSync(ctx.Context(), actorName, msg, forwardTimeout)
	if err != nil {
		ctx.Logger().Warnf("dc-gateway: cross-DC lookup failed for actor=%s: %v", actorName, err)
		ctx.Response(&messages.Account{})
		return
	}
	ctx.Response(reply)
}

// AccountEntity represents the actor implementation using Go structs with persistence
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
	switch msg := ctx.Message().(type) {
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
			ctx.Response(&messages.Account{
				AccountID:      state.AccountID(),
				AccountBalance: state.Balance(),
			})
			return
		}

		// get the data
		accountID := msg.AccountID
		balance := msg.AccountBalance

		// set the new values
		state.SetBalance(balance)
		state.SetCreatedAt(time.Now())

		// update the in-memory state
		x.state = state

		ctx.Response(&messages.Account{
			AccountID:      accountID,
			AccountBalance: state.Balance(),
		})

	case *messages.CreditAccount:
		ctx.Logger().Info("crediting balance...")
		state := x.state

		// get the data
		accountID := msg.AccountID
		balance := msg.Balance

		newBalance := state.Balance() + balance
		state.SetBalance(newBalance)

		// update the in-memory state
		x.state = state
		ctx.Response(&messages.Account{
			AccountID:      accountID,
			AccountBalance: state.Balance(),
		})

	case *messages.GetAccount:
		ctx.Logger().Info("get account...")
		state := x.state
		ctx.Response(&messages.Account{
			AccountID:      msg.AccountID,
			AccountBalance: state.Balance(),
		})
	default:
		ctx.Unhandled()
	}
}

// PostStop is used to free-up resources when the actor stops
func (x *AccountEntity) PostStop(ctx *actor.Context) error {
	underlying := x.state
	if underlying == nil {
		return nil
	}
	return x.storage.WriteState(ctx.Context(), underlying.AccountID(), underlying)
}
