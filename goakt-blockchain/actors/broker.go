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
	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/messages"
)

// Broker is the actor in charge of the transactions that are waiting to be
// included in a block.
type Broker struct {
	pending []chain.Transaction
}

var _ goakt.Actor = (*Broker)(nil)

func (x *Broker) PreStart(*goakt.Context) error {
	x.pending = nil
	return nil
}

func (x *Broker) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goakt.PostStart:
		ctx.Logger().Infof("%s started", ctx.Self().Name())
	case *messages.AddTransaction:
		x.pending = append(x.pending, msg.Transaction)
		ctx.Logger().Infof("Added transaction: %s -> %s (%v coins)",
			msg.Transaction.Sender, msg.Transaction.Recipient, msg.Transaction.Value)
	case *messages.GetTransactions:
		items := make([]chain.Transaction, len(x.pending))
		copy(items, x.pending)
		ctx.Response(&messages.PendingTransactions{Items: items})
	case *messages.ClearTransactions:
		x.pending = nil
		ctx.Logger().Info("Pending transactions cleared")
	default:
		ctx.Unhandled()
	}
}

func (x *Broker) PostStop(*goakt.Context) error {
	return nil
}
