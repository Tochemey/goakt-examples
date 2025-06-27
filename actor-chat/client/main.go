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
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	actors "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/address"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"
	"github.com/travisjeffery/go-dynaport"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/tochemey/goakt-examples/v2/internal/chatpb"
)

func main() {
	ctx := context.Background()
	host := "0.0.0.0"
	ports := dynaport.Get(1)
	port := ports[0]
	userName := fmt.Sprintf("%s@%s", os.Getenv("USER"), time.Now().Format("20060102150405"))
	var err error

	logger := log.New(log.InfoLevel, os.Stdout)

	actorSystem, err := actors.NewActorSystem(
		"ChatSystem",
		actors.WithRemote(remote.NewConfig(host, port)),
		actors.WithLogger(logger))

	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// start the actor system
	if err := actorSystem.Start(ctx); err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// wait for the actor system properly start
	// prior to spawn actors
	time.Sleep(time.Second)

	serverAddress := address.New("ChatServer", actorSystem.Name(), host, 4000)
	var client *actors.PID

	// spawn the client actor
	if client, err = actorSystem.Spawn(
		ctx,
		"ChatClient",
		NewClient(userName, serverAddress),
		actors.WithSupervisor(
			actors.NewSupervisor(
				actors.WithStrategy(actors.OneForOneStrategy),
				actors.WithAnyErrorDirective(actors.ResumeDirective),
			))); err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// wait for the actor to properly start
	time.Sleep(time.Second)

	fmt.Println("Type 'quit' and press return to exit.")
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			logger.Errorf("failed to read from stdin: %v", err)
			continue
		}

		input = strings.TrimSpace(input)
		msg := &chatpb.Message{
			UserName: userName,
			Content:  input,
		}

		// send quit to stop the chat
		if strings.ToLower(msg.GetContent()) == "quit" {
			break
		}

		if err := client.RemoteTell(ctx, serverAddress, msg); err != nil {
			logger.Error(err)
			break
		}
	}

	err = client.RemoteTell(ctx, serverAddress, &chatpb.Disconnect{})
	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type Client struct {
	userName string
	serverID *address.Address
	logger   log.Logger
}

// enforce compilation error
var _ actors.Actor = (*Client)(nil)

// NewClient creates an instance of Client
func NewClient(userName string, serverID *address.Address) *Client {
	return &Client{
		userName: userName,
		serverID: serverID,
	}
}

func (c *Client) PreStart(*actors.Context) error {
	return nil
}

func (c *Client) Receive(ctx *actors.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		c.logger = ctx.Logger()
		c.logger.Info("Chat Client successfully started")
		ctx.RemoteTell(c.serverID, &chatpb.Connect{
			UserName: c.userName,
		})
	case *chatpb.Message:
		c.logger.Infof("Received message: %s", protojson.Format(msg))
	default:
		ctx.Unhandled()
	}
}

func (c *Client) PostStop(*actors.Context) error {
	c.logger.Info("Chat Client successfully stopped")
	return nil
}
