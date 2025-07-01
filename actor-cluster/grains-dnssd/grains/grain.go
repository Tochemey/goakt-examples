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

package grains

import (
	"context"
	"time"

	"github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/log"
	"go.uber.org/atomic"

	"github.com/tochemey/goakt-examples/v2/actor-cluster/grains-dnssd/domain"
	"github.com/tochemey/goakt-examples/v2/actor-cluster/grains-dnssd/persistence"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

var zeroTime = time.Time{}

type AccountGrain struct {
	state   *atomic.Pointer[domain.Account]
	storage persistence.Store
	logger  log.Logger
}

var _ actor.Grain = (*AccountGrain)(nil)

func NewAccountGrain() *AccountGrain {
	return &AccountGrain{
		state: atomic.NewPointer[domain.Account](new(domain.Account)),
	}
}

func (x *AccountGrain) OnActivate(ctx context.Context, props *actor.GrainProps) error {
	accountID := props.Identity().Name()
	x.state = atomic.NewPointer[domain.Account](domain.NewAccount(accountID, 0, zeroTime))
	actorSystem := props.ActorSystem()
	x.storage = actorSystem.Extension(persistence.StateStoreID).(persistence.Store)
	recoveredState, err := x.storage.GetState(ctx, accountID)
	if err != nil {
		return err
	}

	x.state.Store(recoveredState)
	x.logger = actorSystem.Logger()
	return nil
}

func (x *AccountGrain) OnReceive(ctx *actor.GrainContext) {
	switch msg := ctx.Message().(type) {
	case *samplepb.CreateAccount:
		x.logger.Info("creating account by setting the balance...")
		state := x.state.Load()

		if !state.CreatedAt().Equal(zeroTime) {
			x.logger.Infof("account=%s has been created already", state.AccountID())
			// TODO: we can return an error here
			ctx.NoErr()
			return
		}

		accountID := msg.GetAccountId()
		balance := msg.GetAccountBalance()

		state.SetBalance(balance)
		state.SetCreatedAt(time.Now())

		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: state.Balance(),
		})

	case *samplepb.CreditAccount:
		x.logger.Info("crediting balance...")
		state := x.state.Load()

		accountID := msg.GetAccountId()
		balance := msg.GetBalance()

		newBalance := state.Balance() + balance
		state.SetBalance(newBalance)

		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: state.Balance(),
		})

	case *samplepb.GetAccount:
		x.logger.Info("get account...")
		state := x.state.Load()

		ctx.Response(&samplepb.Account{
			AccountId:      msg.GetAccountId(),
			AccountBalance: state.Balance(),
		})
	default:
		ctx.Unhandled()
	}
}

func (x *AccountGrain) OnDeactivate(ctx context.Context, _ *actor.GrainProps) error {
	underlying := x.state.Load()
	return x.storage.WriteState(ctx, underlying)
}
