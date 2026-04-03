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

package grains

import (
	"context"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"

	"github.com/tochemey/goakt-examples/v2/goakt-grains-cluster/grains-dnssd/domain"
	"github.com/tochemey/goakt-examples/v2/goakt-grains-cluster/grains-dnssd/persistence"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

var zeroTime = time.Time{}

type AccountGrain struct {
	state   *domain.Account
	storage persistence.Store
	logger  log.Logger
}

var _ actor.Grain = (*AccountGrain)(nil)

func NewAccountGrain() *AccountGrain {
	return &AccountGrain{
		state: new(domain.Account),
	}
}

func (x *AccountGrain) OnActivate(ctx context.Context, props *actor.GrainProps) error {
	accountID := props.Identity().Name()
	x.state = domain.NewAccount(accountID, 0, zeroTime)
	actorSystem := props.ActorSystem()
	x.storage = actorSystem.Extension(persistence.StateStoreID).(persistence.Store)
	recoveredState, err := x.storage.GetState(ctx, accountID)
	if err != nil {
		return err
	}

	x.state = recoveredState
	x.logger = actorSystem.Logger()
	return nil
}

func (x *AccountGrain) OnReceive(ctx *actor.GrainContext) {
	switch msg := ctx.Message().(type) {
	case *samplepb.CreateAccount:
		x.logger.Info("creating account by setting the balance...")

		if !x.state.CreatedAt().Equal(zeroTime) {
			x.logger.Infof("account=%s has been created already", x.state.AccountID())
			// TODO: we can return an error here
			ctx.NoErr()
			return
		}

		accountID := msg.GetAccountId()
		balance := msg.GetAccountBalance()

		x.state.SetBalance(balance)
		x.state.SetCreatedAt(time.Now())

		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: x.state.Balance(),
		})

	case *samplepb.CreditAccount:
		x.logger.Info("crediting balance...")

		accountID := msg.GetAccountId()
		balance := msg.GetBalance()

		newBalance := x.state.Balance() + balance
		x.state.SetBalance(newBalance)

		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: x.state.Balance(),
		})

	case *samplepb.GetAccount:
		x.logger.Info("get account...")

		ctx.Response(&samplepb.Account{
			AccountId:      msg.GetAccountId(),
			AccountBalance: x.state.Balance(),
		})
	default:
		ctx.Unhandled()
	}
}

func (x *AccountGrain) OnDeactivate(ctx context.Context, _ *actor.GrainProps) error {
	return x.storage.WriteState(ctx, x.state)
}
