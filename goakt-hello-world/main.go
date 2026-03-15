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
	"os"
	"os/signal"
	"syscall"
	"time"

	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"

	hellopb "github.com/tochemey/goakt-examples/v2/internal/helloworldpb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.DefaultLogger

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.NewActorSystem(
		"HelloWorld",
		goakt.WithLogger(logger),
		goakt.WithActorInitMaxRetries(3))

	// start the actor system
	if err := actorSystem.Start(ctx); err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// create a Hello actor
	pid, err := actorSystem.Spawn(ctx, "Hello", NewHelloWorld())
	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// send an SayHello message to the actor and expect a response
	response, err := goakt.Ask(ctx, pid, new(hellopb.SayHello), time.Second)
	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	switch response.(type) {
	case *hellopb.SayHi:
		logger.Info("received SayHi from actor")
	default:
		logger.Fatal("unexpected response from actor")
	}

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type HelloWorld struct{}

var _ goakt.Actor = (*HelloWorld)(nil)

// NewHelloWorld creates an instance
func NewHelloWorld() *HelloWorld {
	return &HelloWorld{}
}

func (x *HelloWorld) PreStart(*goakt.Context) error { return nil }

func (x *HelloWorld) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *hellopb.SayHello:
		ctx.Response(new(hellopb.SayHi))
	default:
		ctx.Unhandled()
	}
}

func (x *HelloWorld) PostStop(*goakt.Context) error { return nil }
