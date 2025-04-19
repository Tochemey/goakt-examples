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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/log"
	"google.golang.org/protobuf/proto"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.DefaultLogger

	stateStore := NewMemoryStore()

	// connect to the storage
	if err := stateStore.Connect(ctx); err != nil {
		logger.Panic(err)
	}

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.
		NewActorSystem(
			"AccountsSystem",
			goakt.WithPassivation(time.Minute),
			goakt.WithLogger(logger),
			goakt.WithExtensions(stateStore),
		)

	// start the actor system
	if err := actorSystem.Start(ctx); err != nil {
		logger.Panic(err)
	}

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// create an actor
	entityID := uuid.NewString()
	pid, err := actorSystem.Spawn(ctx, entityID, NewAccountEntity())
	if err != nil {
		logger.Panic(err)
	}

	var command proto.Message

	// create the account
	command = &samplepb.CreateAccount{
		AccountId:      entityID,
		AccountBalance: 500.00,
	}

	// send the message to the actor and wait for the response
	response, err := goakt.Ask(ctx, pid, command, time.Minute)
	if err != nil {
		logger.Panic(err)
	}

	account := response.(*samplepb.Account)
	logger.Infof("current balance on opening: %v", account.GetAccountBalance())

	// fetch the account from the store and compare the outcome
	fromStore, err := stateStore.GetLatestState(ctx, entityID)
	if err != nil {
		logger.Panic(err)
	}

	logger.Infof("current balance from store: %v", fromStore.GetAccountBalance())

	// send another command to credit the balance
	command = &samplepb.CreditAccount{
		AccountId: entityID,
		Balance:   250,
	}

	// send the message to the actor and wait for the response
	response, err = goakt.Ask(ctx, pid, command, time.Minute)
	if err != nil {
		logger.Panic(err)
	}

	account = response.(*samplepb.Account)
	logger.Infof("current balance after a credit of 250: %v", account.GetAccountBalance())

	// fetch the account from the store and compare the outcome
	fromStore, err = stateStore.GetLatestState(ctx, entityID)
	if err != nil {
		logger.Panic(err)
	}

	logger.Infof("current balance from store: %v", fromStore.GetAccountBalance())

	// Wait for the actor to passivate and create a new instance of the actor and fetch its state.
	time.Sleep(2 * time.Minute)

	// here we create a new instance of the entity and we expect to recover its previous state from store
	pid, err = actorSystem.Spawn(ctx, entityID, NewAccountEntity())
	if err != nil {
		logger.Panic(err)
	}

	command = &samplepb.GetAccount{AccountId: entityID}
	// send the message to the actor and wait for the response
	response, err = goakt.Ask(ctx, pid, command, time.Minute)
	if err != nil {
		logger.Panic(err)
	}

	account = response.(*samplepb.Account)
	logger.Infof("current balance after (re)start: %v", account.GetAccountBalance())

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
