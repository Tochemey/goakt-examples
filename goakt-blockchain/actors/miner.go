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

// Miner executes the proof-of-work. It is a state machine with a ready and a
// busy behavior:
//
//   - ready: a Mine message starts the computation and switches to busy.
//   - busy: further Mine messages are rejected until a Ready message arrives.
//
// The computation runs off the mailbox goroutine through PipeTo, which
// delivers the ProofFound result to the sender when it completes, so the
// miner remains responsive while mining.
type Miner struct{}

var _ goakt.Actor = (*Miner)(nil)

func (x *Miner) PreStart(*goakt.Context) error {
	return nil
}

// Receive is the ready behavior: the miner accepts mining requests.
func (x *Miner) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goakt.PostStart:
		ctx.Logger().Infof("%s started and ready to mine", ctx.Self().Name())
	case *messages.Mine:
		lastHash := msg.LastHash
		ctx.Logger().Infof("Mining on top of hash %s...", lastHash)
		// run the proof-of-work outside the mailbox and pipe the result back
		// to the requester (the node actor)
		ctx.PipeTo(ctx.Sender(), func() (any, error) {
			return &messages.ProofFound{LastHash: lastHash, Proof: chain.ProofOfWork(lastHash)}, nil
		})
		// no more mining until the node reports the block was added
		ctx.Become(x.Busy)
	case *messages.Validate:
		ctx.Response(&messages.ProofValidity{Valid: chain.ValidProof(msg.Hash, msg.Proof)})
	case *messages.Ready:
		// already ready, nothing to do
	default:
		ctx.Unhandled()
	}
}

// Busy is the behavior while a proof-of-work is in progress. Validation
// requests are still served, but new mining requests are turned down.
func (x *Miner) Busy(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.Mine:
		ctx.Logger().Warn("Already mining, request dropped")
	case *messages.Validate:
		ctx.Response(&messages.ProofValidity{Valid: chain.ValidProof(msg.Hash, msg.Proof)})
	case *messages.Ready:
		ctx.Logger().Info("Block added, ready to mine again")
		ctx.UnBecome()
	default:
		ctx.Unhandled()
	}
}

func (x *Miner) PostStop(*goakt.Context) error {
	return nil
}
