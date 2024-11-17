/*
 * MIT License
 *
 * Copyright (c) 2022-2024 Arsene Tochemey Gandote
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
	"context"

	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	"github.com/tochemey/goakt-examples/v2/samplepb"
)

// AccountEntity represents the immutable implementation of Actor
type AccountEntity struct {
	accountID string
	balance   float64
	created   bool
}

// enforce compilation error
var _ goakt.Actor = (*AccountEntity)(nil)

// NewAccountEntity creates an instance of AccountEntity
func NewAccountEntity() *AccountEntity {
	return &AccountEntity{}
}

// PreStart is used to pre-set initial values for the actor
func (p *AccountEntity) PreStart(context.Context) error {
	return nil
}

// Receive handles the messages sent to the actor
func (p *AccountEntity) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		// set the account ID
		p.accountID = ctx.Self().Name()
	case *samplepb.CreateAccount:
		ctx.Self().Logger().Info("creating account by setting the balance...")
		// check whether the create operation has been done already
		if p.created {
			ctx.Self().Logger().Infof("account=%s has been created already", p.accountID)
			return
		}
		// get the data
		accountID := msg.GetAccountId()
		balance := msg.GetAccountBalance()
		// first check whether the accountID is mine
		if p.accountID == accountID {
			p.balance = balance
			p.created = true
			// here we are handling just an ask
			ctx.Response(&samplepb.Account{
				AccountId:      accountID,
				AccountBalance: p.balance,
			})
		}
	case *samplepb.CreditAccount:
		ctx.Self().Logger().Info("crediting balance...")
		// get the data
		accountID := msg.GetAccountId()
		balance := msg.GetBalance()
		// first check whether the accountID is mine
		if p.accountID == accountID {
			p.balance += balance
			ctx.Response(&samplepb.Account{
				AccountId:      accountID,
				AccountBalance: p.balance,
			})
		}
	case *samplepb.GetAccount:
		ctx.Self().Logger().Info("get account...")
		// get the data
		ctx.Response(&samplepb.Account{
			AccountId:      msg.GetAccountId(),
			AccountBalance: p.balance,
		})
	default:
		ctx.Unhandled()
	}
}

// PostStop is used to free-up resources when the actor stops
func (p *AccountEntity) PostStop(context.Context) error {
	p.created = false
	p.balance = 0
	return nil
}
