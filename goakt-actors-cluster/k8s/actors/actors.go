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
	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

// Account represents the immutable implementation of Actor
type Account struct {
	accountID string
	balance   float64
	created   bool
}

// enforce compilation error
var _ actor.Actor = (*Account)(nil)

func NewAccount() *Account {
	return &Account{}
}

// PreStart is used to pre-set initial values for the actor
func (x *Account) PreStart(*actor.Context) error {
	return nil
}

// Receive handles the messages sent to the actor
func (x *Account) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		x.accountID = ctx.Self().Name()
		ctx.Logger().Infof("account entity=(%s) successfully started", x.accountID)
	case *samplepb.CreateAccount:
		ctx.Logger().Info("creating account by setting the balance...")
		if x.created {
			ctx.Logger().Infof("account=%s has been created already", x.accountID)
			ctx.Unhandled()
			return
		}

		accountID := msg.GetAccountId()
		balance := msg.GetAccountBalance()
		x.balance = balance
		x.created = true
		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: x.balance,
		})
	case *samplepb.CreditAccount:
		ctx.Logger().Info("crediting balance...")
		balance := msg.GetBalance()
		x.balance += balance
		ctx.Response(&samplepb.Account{
			AccountId:      msg.GetAccountId(),
			AccountBalance: x.balance,
		})
	case *samplepb.GetAccount:
		ctx.Logger().Info("get account...")
		accountID := msg.GetAccountId()
		ctx.Response(&samplepb.Account{
			AccountId:      accountID,
			AccountBalance: x.balance,
		})

	default:
		ctx.Unhandled()
	}
}

// PostStop is used to free-up resources when the actor stops
func (x *Account) PostStop(*actor.Context) error {
	x.created = false
	x.balance = 0.0
	return nil
}
