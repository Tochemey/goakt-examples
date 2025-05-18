/*
 * MIT License
 *
 * Copyright (c) 2022-2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package actors

import (
	"reflect"
	"sync/atomic"
	"time"

	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/goaktpb"

	"github.com/tochemey/goakt-examples/v2/actor-cluster/dnssd/domain"
	"github.com/tochemey/goakt-examples/v2/actor-cluster/dnssd/persistence"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

var zeroTime = time.Time{}

// AccountEntity represents the immutable implementation of Actor
type AccountEntity struct {
	state   *atomic.Pointer[domain.Account]
	storage persistence.Store
}

// enforce compilation error
var _ goakt.Actor = (*AccountEntity)(nil)

// NewAccountEntity creates an instance of AccountEntity
func NewAccountEntity() *AccountEntity {
	return &AccountEntity{}
}

// PreStart is used to pre-set initial values for the actor
func (x *AccountEntity) PreStart(ctx *goakt.Context) error {
	accountID := ctx.ActorName()
	x.storage = ctx.Extension(persistence.MemoryStateStoreID).(persistence.Store)
	recoveredState := x.storage.GetState(ctx.Context(), accountID)
	x.state.Store(recoveredState)
	return nil
}

// Receive handles the messages sent to the actor
func (x *AccountEntity) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		state := x.state.Load()
		if reflect.DeepEqual(state, new(domain.Account)) {
			state.SetCreatedAt(zeroTime)
			state.SetBalance(0)
		}

	case *samplepb.CreateAccount:
		ctx.Self().Logger().Info("creating account by setting the balance...")
		state := x.state.Load()

		// check whether the create operation has been done already
		if !state.CreatedAt().Equal(zeroTime) {
			ctx.Self().Logger().Infof("account=%s has been created already", state.AccountID())
			return
		}

		// get the data
		accountID := msg.GetAccountId()
		balance := msg.GetAccountBalance()

		// set the new values
		state.SetBalance(balance)
		state.SetCreatedAt(time.Now())

		// here we are handling just an ask
		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: state.Balance(),
		})

	case *samplepb.CreditAccount:
		ctx.Self().Logger().Info("crediting balance...")
		state := x.state.Load()

		// get the data
		accountID := msg.GetAccountId()
		balance := msg.GetBalance()

		newBalance := state.Balance() + balance
		state.SetBalance(newBalance)

		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: state.Balance(),
		})

	case *samplepb.GetAccount:
		ctx.Self().Logger().Info("get account...")
		state := x.state.Load()
		// get the data
		ctx.Response(&samplepb.Account{
			AccountId:      msg.GetAccountId(),
			AccountBalance: state.Balance(),
		})
	default:
		ctx.Unhandled()
	}
}

// PostStop is used to free-up resources when the actor stops
func (x *AccountEntity) PostStop(ctx *goakt.Context) error {
	underlying := x.state.Load()
	x.storage.WriteState(ctx.Context(), underlying.AccountID(), underlying)
	return nil
}
