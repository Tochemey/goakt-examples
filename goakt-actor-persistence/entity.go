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

package main

import (
	"context"
	"fmt"

	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/goaktpb"
	"go.uber.org/atomic"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

// AccountEntity represents the immutable implementation of Actor
type AccountEntity struct {
	accountID  string
	created    bool
	stateStore StateStore
	state      *atomic.Pointer[samplepb.Account]
}

// enforce compilation error
var _ goakt.Actor = (*AccountEntity)(nil)

// NewAccountEntity creates an instance of AccountEntity
func NewAccountEntity() *AccountEntity {
	return &AccountEntity{}
}

// PreStart is used to pre-set initial values for the actor
func (entity *AccountEntity) PreStart(ctx *goakt.Context) error {
	entity.state = atomic.NewPointer(new(samplepb.Account))
	entity.stateStore = ctx.Extension("MemoryStore").(StateStore)
	entity.accountID = ctx.ActorName()
	return entity.recoverFromStore(ctx.Context())
}

// Receive handles the messages sent to the actor
func (entity *AccountEntity) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		ctx.Logger().Infof("%s properly started", entity.accountID)
	case *samplepb.CreateAccount:
		ctx.Logger().Info("creating account by setting the balance...")

		if entity.accountID != msg.GetAccountId() {
			ctx.Logger().Infof("account=%s is not mine", entity.accountID)
			ctx.Unhandled()
			return
		}

		if entity.created {
			ctx.Self().Logger().Infof("account=%s has been created already", entity.accountID)
			return
		}

		balance := msg.GetAccountBalance()
		entity.state.Load().AccountBalance = entity.state.Load().GetAccountBalance() + balance

		// persist the actor state
		if err := entity.stateStore.WriteState(ctx.Context(), entity.accountID, entity.state.Load()); err != nil {
			ctx.Err(err)
			return
		}

		ctx.Response(entity.state.Load())
	case *samplepb.CreditAccount:
		ctx.Self().Logger().Info("crediting balance...")

		if entity.accountID != msg.GetAccountId() {
			ctx.Logger().Infof("account=%s is not mine", entity.accountID)
			ctx.Unhandled()
			return
		}

		balance := msg.GetBalance()
		entity.state.Load().AccountBalance = entity.state.Load().GetAccountBalance() + balance

		// persist the actor state
		if err := entity.stateStore.WriteState(ctx.Context(), entity.accountID, entity.state.Load()); err != nil {
			ctx.Err(err)
			return
		}

		ctx.Response(entity.state.Load())
	case *samplepb.GetAccount:
		ctx.Logger().Info("get account...")

		if entity.accountID != msg.GetAccountId() {
			ctx.Logger().Infof("account=%s is not mine", entity.accountID)
			ctx.Unhandled()
			return
		}

		ctx.Response(entity.state.Load())
	default:
		ctx.Unhandled()
	}
}

// PostStop is used to free-up resources when the actor stops
func (entity *AccountEntity) PostStop(ctx *goakt.Context) error {
	return entity.stateStore.WriteState(ctx.Context(), entity.accountID, entity.state.Load())
}

func (entity *AccountEntity) recoverFromStore(ctx context.Context) error {
	latestState, err := entity.stateStore.GetLatestState(ctx, entity.accountID)
	if err != nil {
		return fmt.Errorf("failed to get the latest state: %w", err)
	}

	if latestState != nil {
		entity.state.Store(latestState)
	}

	return nil
}
