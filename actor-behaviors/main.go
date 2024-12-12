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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.DefaultLogger

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.NewActorSystem("AccountsSystem",
		goakt.WithPassivationDisabled(),
		goakt.WithLogger(logger))

	// start the actor system
	_ = actorSystem.Start(ctx)

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// create an actor
	actor := NewAccountActor()
	accountID := uuid.NewString()
	actorRef, _ := actorSystem.Spawn(ctx, accountID, actor)

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// here we ar build an FSM:
	// 1- We perform the authentication
	response, err := goakt.Ask(ctx, actorRef, new(samplepb.Authenticate), time.Second)
	if err != nil {
		logger.Panicf(fmt.Sprintf("failed to authenticate: %v", err))
	}

	// we need to make sure we have successfully authenticated before performing any action
	// One can also add some logic in the actor to always check that before any further processing
	// we leave this out for the sake of the simplicity of the example
	_, authenticated := response.(*samplepb.Authenticated)
	if !authenticated {
		// stop the actor system
		_ = actorSystem.Stop(ctx)
		os.Exit(0)
	}

	logger.Infof("Authentication was successfully")

	// 2- we simulate an account creation just to set the initial account balance of 1000
	response, err = goakt.Ask(ctx, actorRef, &samplepb.CreateAccount{
		AccountId:      accountID,
		AccountBalance: 1000.00,
	}, time.Second)

	if err != nil {
		logger.Panicf(fmt.Sprintf("failed to create account: %v", err))
	}

	accountCreated := response.(*samplepb.AccountCreated)
	logger.Infof("Account created with a balance of: %v", accountCreated.GetAccountBalance())

	// 3- we simulate a credit of 500 into the account
	response, err = goakt.Ask(ctx, actorRef, &samplepb.CreditAccount{
		AccountId: accountID,
		Balance:   500.00,
	}, time.Second)

	if err != nil {
		logger.Panicf(fmt.Sprintf("failed to credit account: %v", err))
	}

	accountCredited := response.(*samplepb.AccountCredited)
	logger.Infof("Account credited and the new balance of: %v", accountCredited.GetAccountBalance())

	// 3- we simulate a credit of 250 into the account
	response, err = goakt.Ask(ctx, actorRef, &samplepb.DebitAccount{
		AccountId: accountID,
		Balance:   250.00,
	}, time.Second)

	if err != nil {
		logger.Panicf(fmt.Sprintf("failed to debit account: %v", err))
	}

	accountDebited := response.(*samplepb.AccountDebited)
	logger.Infof("Account debited and the new balance of: %v", accountDebited.GetAccountBalance())

	// 4- let us fetch the account information
	response, err = goakt.Ask(ctx, actorRef, &samplepb.GetAccount{AccountId: accountID}, time.Second)
	if err != nil {
		logger.Panicf(fmt.Sprintf("failed to get account: %v", err))
	}

	account := response.(*samplepb.Account)
	logger.Infof("Account current balance: %v", account.GetAccountBalance())

	// 5- Logout
	if err := goakt.Tell(ctx, actorRef, new(samplepb.Logout)); err != nil {
		logger.Panicf(fmt.Sprintf("failed to logout: %v", err))
	}

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

// AccountActor implements goakt.AccountActor
type AccountActor struct {
	accountID string
	balance   float64
}

var _ goakt.Actor = (*AccountActor)(nil)

func NewAccountActor() *AccountActor {
	return &AccountActor{}
}

func (actor *AccountActor) PreStart(context.Context) error {
	return nil
}

func (actor *AccountActor) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		// set the account ID
		actor.accountID = ctx.Self().Name()
	case *samplepb.Authenticate:
		// here we respond to the caller that we have successfully authenticated
		ctx.Response(new(samplepb.Authenticated))
		// then we switch to the Authenticated mode
		// in this mode we can process request
		ctx.Become(actor.Authenticated)
	case *samplepb.Logout:
		// we gracefully shut down
		ctx.Shutdown()
	default:
		ctx.Unhandled()
	}
}

// Authenticated defines the actor behavior when the actor has been authenticated
func (actor *AccountActor) Authenticated(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *samplepb.CreateAccount:
		// set the balance
		actor.balance = msg.GetAccountBalance()
		// we respond that the account has been created
		ctx.Response(&samplepb.AccountCreated{
			AccountId:      actor.accountID,
			AccountBalance: actor.balance,
		})
		// then we switch to credit mode because in this example we need to credit the account
		ctx.Become(actor.CreditState)
	default:
		ctx.Unhandled()
	}
}

// CreditState defines the actor behavior for crediting accounts
func (actor *AccountActor) CreditState(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *samplepb.CreditAccount:
		// set the balance
		actor.balance += msg.GetBalance()
		// we respond that the account has been credited
		ctx.Response(&samplepb.AccountCredited{
			AccountId:      actor.accountID,
			AccountBalance: actor.balance,
		})
		// then we move into a debit mode. Here we use BecomeStacked
		ctx.BecomeStacked(actor.DebitState)
	case *samplepb.GetAccount:
		// respond with the account information
		ctx.Response(&samplepb.Account{
			AccountId:      actor.accountID,
			AccountBalance: actor.balance,
		})
		// then switch back to the default behavior which is the Receive
		ctx.UnBecome()
	default:
		ctx.Unhandled()
	}
}

// DebitState defines the actor behavior for debiting accounts
func (actor *AccountActor) DebitState(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *samplepb.DebitAccount:
		// set the balance
		actor.balance -= msg.GetBalance()
		// we respond that the account has been credited
		ctx.Response(&samplepb.AccountDebited{
			AccountId:      actor.accountID,
			AccountBalance: actor.balance,
		})
		// then we move back to the Credit state
		// refer to the documentation of UnBecomeStacked
		ctx.UnBecomeStacked()
	default:
		ctx.Unhandled()
	}
}

func (actor *AccountActor) PostStop(context.Context) error {
	actor.balance = 0
	return nil
}
