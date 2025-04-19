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

package main

import (
	"context"
	"fmt"

	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/goaktpb"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

// AccountEntity represents the immutable implementation of Actor
type AccountEntity struct {
	accountID  string
	created    bool
	stateStore StateStore
	state      *samplepb.Account
}

// enforce compilation error
var _ goakt.Actor = (*AccountEntity)(nil)

// NewAccountEntity creates an instance of AccountEntity
func NewAccountEntity() *AccountEntity {
	return &AccountEntity{}
}

// PreStart is used to pre-set initial values for the actor
func (entity *AccountEntity) PreStart(context.Context) error {
	return nil
}

// Receive handles the messages sent to the actor
func (entity *AccountEntity) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		// initialize some the entity properties
		entity.accountID = ctx.Self().Name()
		entity.state = new(samplepb.Account)
		entity.stateStore = ctx.Extension("MemoryStore").(StateStore)

		// recover state from state store
		if err := entity.recoverFromStore(ctx.Context()); err != nil {
			ctx.Err(err)
			return
		}

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
		entity.state.AccountBalance = entity.state.GetAccountBalance() + balance

		// persist the actor state
		if err := entity.stateStore.WriteState(ctx.Context(), entity.accountID, entity.state); err != nil {
			ctx.Err(err)
			return
		}

		ctx.Response(entity.state)
	case *samplepb.CreditAccount:
		ctx.Self().Logger().Info("crediting balance...")

		if entity.accountID != msg.GetAccountId() {
			ctx.Logger().Infof("account=%s is not mine", entity.accountID)
			ctx.Unhandled()
			return
		}

		balance := msg.GetBalance()
		entity.state.AccountBalance = entity.state.GetAccountBalance() + balance

		// persist the actor state
		if err := entity.stateStore.WriteState(ctx.Context(), entity.accountID, entity.state); err != nil {
			ctx.Err(err)
			return
		}

		ctx.Response(entity.state)
	case *samplepb.GetAccount:
		ctx.Logger().Info("get account...")

		if entity.accountID != msg.GetAccountId() {
			ctx.Logger().Infof("account=%s is not mine", entity.accountID)
			ctx.Unhandled()
			return
		}

		ctx.Response(entity.state)
	default:
		ctx.Unhandled()
	}
}

// PostStop is used to free-up resources when the actor stops
func (entity *AccountEntity) PostStop(ctx context.Context) error {
	return entity.stateStore.WriteState(ctx, entity.accountID, entity.state)
}

func (entity *AccountEntity) recoverFromStore(ctx context.Context) error {
	latestState, err := entity.stateStore.GetLatestState(ctx, entity.accountID)
	if err != nil {
		return fmt.Errorf("failed to get the latest state: %w", err)
	}

	if latestState != nil {
		entity.state = latestState
	}

	return nil
}
