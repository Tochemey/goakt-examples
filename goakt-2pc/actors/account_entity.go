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
	"errors"
	"reflect"
	"time"

	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-2pc/domain"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/persistence"
)

var zeroTime = time.Time{}

// ErrInsufficientFunds is returned when debit would result in negative balance
var ErrInsufficientFunds = errors.New("insufficient funds")

// preparedTransfer holds the state of a prepared transaction
type preparedTransfer struct {
	transferID string
	amount     float64
	isDebit    bool
}

// AccountEntity represents the actor implementation for account operations
type AccountEntity struct {
	state    *domain.Account
	storage  persistence.Store
	prepared map[string]*preparedTransfer // transferID -> prepared state
}

var _ actor.Actor = (*AccountEntity)(nil)

// NewAccountEntity creates an instance of AccountEntity
func NewAccountEntity() *AccountEntity {
	return &AccountEntity{}
}

// PreStart is used to pre-set initial values for the actor
func (x *AccountEntity) PreStart(ctx *actor.Context) error {
	accountID := ctx.ActorName()
	x.prepared = make(map[string]*preparedTransfer)
	x.storage = ctx.Extension(persistence.PostgresStateStoreID).(persistence.Store)
	latestState, err := x.storage.GetAccountState(ctx.Context(), accountID)
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

		if !state.CreatedAt().Equal(zeroTime) {
			ctx.Logger().Infof("account=%s has been created already", state.AccountID())
			ctx.Response(&messages.Account{
				AccountID:      state.AccountID(),
				AccountBalance: state.Balance(),
			})
			return
		}

		state.SetBalance(msg.AccountBalance)
		state.SetCreatedAt(time.Now())
		x.state = state

		ctx.Response(&messages.Account{
			AccountID:      msg.AccountID,
			AccountBalance: state.Balance(),
		})

	case *messages.PrepareTransfer:
		ctx.Logger().Infof("preparing transfer %s...", msg.TransferID)
		x.handlePrepareTransfer(ctx, msg)

	case *messages.CommitTransfer:
		ctx.Logger().Infof("committing transfer %s...", msg.TransferID)
		x.handleCommitTransfer(ctx, msg)

	case *messages.AbortTransfer:
		ctx.Logger().Infof("aborting transfer %s...", msg.TransferID)
		x.handleAbortTransfer(ctx, msg)

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

func (x *AccountEntity) handlePrepareTransfer(ctx *actor.ReceiveContext, msg *messages.PrepareTransfer) {
	state := x.state

	// Check if already prepared this transfer
	if _, exists := x.prepared[msg.TransferID]; exists {
		ctx.Response(&messages.VoteYes{TransferID: msg.TransferID, AccountID: state.AccountID()})
		return
	}

	ctx.Logger().Infof("Balance: %f", state.Balance())

	// Validate the operation
	if msg.IsDebit {
		// Debit (source account)
		if state.Balance() < msg.Amount {
			ctx.Response(&messages.VoteNo{
				TransferID: msg.TransferID,
				AccountID:  state.AccountID(),
				Reason:     "insufficient funds",
			})
			return
		}
	}

	// Prepare: lock the resources
	x.prepared[msg.TransferID] = &preparedTransfer{
		transferID: msg.TransferID,
		amount:     msg.Amount,
		isDebit:    msg.IsDebit,
	}

	ctx.Logger().Infof("account %s voted YES for transfer %s", state.AccountID(), msg.TransferID)
	ctx.Response(&messages.VoteYes{TransferID: msg.TransferID, AccountID: state.AccountID()})
}

func (x *AccountEntity) handleCommitTransfer(ctx *actor.ReceiveContext, msg *messages.CommitTransfer) {
	prepared, exists := x.prepared[msg.TransferID]
	if !exists {
		// Already committed or never prepared - idempotent
		ctx.Response(&messages.Account{
			AccountID:      x.state.AccountID(),
			AccountBalance: x.state.Balance(),
		})
		return
	}

	state := x.state

	// Apply the change
	if prepared.isDebit {
		newBalance := state.Balance() - prepared.amount
		state.SetBalance(newBalance)
	} else {
		newBalance := state.Balance() + prepared.amount
		state.SetBalance(newBalance)
	}

	x.state = state

	// Release the lock
	delete(x.prepared, msg.TransferID)

	ctx.Logger().Infof("account %s committed transfer %s, new balance: %f", state.AccountID(), msg.TransferID, state.Balance())
	ctx.Response(&messages.Account{
		AccountID:      state.AccountID(),
		AccountBalance: state.Balance(),
	})
}

func (x *AccountEntity) handleAbortTransfer(ctx *actor.ReceiveContext, msg *messages.AbortTransfer) {
	// Release any lock for this transfer
	if _, exists := x.prepared[msg.TransferID]; exists {
		delete(x.prepared, msg.TransferID)
		ctx.Logger().Infof("account %s aborted transfer %s", x.state.AccountID(), msg.TransferID)
	}

	ctx.Response(&messages.Account{
		AccountID:      x.state.AccountID(),
		AccountBalance: x.state.Balance(),
	})
}

// PostStop is used to free-up resources when the actor stops
func (x *AccountEntity) PostStop(ctx *actor.Context) error {
	underlying := x.state
	if underlying == nil {
		return nil
	}
	return x.storage.WriteAccountState(ctx.Context(), underlying.AccountID(), underlying)
}
