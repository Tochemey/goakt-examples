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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"google.golang.org/protobuf/proto"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.DiscardLogger

	stateStore := NewMemoryStore()

	// connect to the storage
	if err := stateStore.Connect(ctx); err != nil {
		logger.Panic(err)
	}

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := actor.
		NewActorSystem(
			"AccountsSystem",
			actor.WithLogger(logger),
			actor.WithExtensions(stateStore),
		)

	// start the actor system
	if err := actorSystem.Start(ctx); err != nil {
		logger.Panic(err)
	}

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// register the grain
	grain := &Grain{}

	accountID := uuid.NewString()
	identity, err := actorSystem.GrainIdentity(ctx, accountID, func(ctx context.Context) (actor.Grain, error) {
		return grain, nil
	})

	var command proto.Message

	// create the account
	command = &samplepb.CreateAccount{
		AccountId:      accountID,
		AccountBalance: 500.00,
	}

	response, err := actorSystem.AskGrain(ctx, identity, command, time.Second)
	if err != nil {
		logger.Fatal(err)
	}

	account := response.(*samplepb.Account)
	fmt.Printf("current balance on opening: %v\n", account.GetAccountBalance())

	// fetch the account from the store and compare the outcome
	fromStore, err := stateStore.GetLatestState(ctx, accountID)
	if err != nil {
		logger.Fatal(err)
	}

	fmt.Printf("current balance from store: %v\n", fromStore.GetAccountBalance())

	// send another command to credit the balance
	command = &samplepb.CreditAccount{
		AccountId: accountID,
		Balance:   250,
	}

	// send the message to the actor and wait for the response
	response, err = actorSystem.AskGrain(ctx, identity, command, time.Second)
	if err != nil {
		logger.Fatal(err)
	}

	account = response.(*samplepb.Account)
	fmt.Printf("current balance after a credit of 250: %v\n", account.GetAccountBalance())

	// fetch the account from the store and compare the outcome
	fromStore, err = stateStore.GetLatestState(ctx, accountID)
	if err != nil {
		logger.Fatal(err)
	}

	fmt.Printf("current balance from store: %v\n", fromStore.GetAccountBalance())

	// Deactivate the grain
	err = actorSystem.TellGrain(ctx, identity, &actor.PoisonPill{})
	if err != nil {
		logger.Fatal(err)
	}

	fmt.Println("waiting for a minute before reactivating the grain. This is just to simulate a long deactivation period...")
	time.Sleep(1 * time.Minute)

	command = &samplepb.GetAccount{AccountId: accountID}
	// send the message to the actor and wait for the response
	response, err = actorSystem.AskGrain(ctx, identity, command, time.Second)
	if err != nil {
		logger.Fatal(err)
	}

	account = response.(*samplepb.Account)
	fmt.Printf("current balance after (re)activation: %v\n", account.GetAccountBalance())

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// disconnect the event store
	_ = stateStore.Disconnect(ctx)
	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}
