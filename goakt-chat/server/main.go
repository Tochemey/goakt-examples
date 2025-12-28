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
	"sync"
	"syscall"
	"time"

	actors "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/address"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"
	"github.com/tochemey/goakt/v3/supervisor"

	"github.com/tochemey/goakt-examples/v2/internal/chatpb"
)

func main() {
	ctx := context.Background()
	host := "0.0.0.0"
	port := 4000

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

	// spawn the server actor
	if _, err := actorSystem.Spawn(
		ctx,
		"ChatServer",
		NewServer(),
		actors.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			))); err != nil {
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

type Server struct {
	logger  log.Logger
	clients *sync.Map
	users   *sync.Map
}

// enforce compilation error
var _ actors.Actor = (*Server)(nil)

// NewServer creates an instance of Server
func NewServer() *Server {
	return &Server{
		clients: new(sync.Map),
		users:   new(sync.Map),
	}
}

func (s *Server) PreStart(*actors.Context) error {
	s.users.Clear()
	s.clients.Clear()
	return nil
}

func (s *Server) Receive(ctx *actors.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		s.logger = ctx.Logger()
		s.logger.Info("Chat Server successfully started")
	case *chatpb.Connect:
		sender := ctx.RemoteSender()
		if _, ok := s.clients.Load(sender.String()); ok {
			s.logger.Warnf("client=(%s) already connected", sender.String())
			return
		}

		if _, ok := s.users.Load(sender.String()); ok {
			s.logger.Warnf("user=(%s) already connected", sender.String())
			return
		}

		s.clients.Store(sender.String(), sender)
		s.users.Store(sender.String(), msg.GetUserName())
		s.logger.Infof("Client=[sender=(%s), username=(%s)] connected", sender.String(), msg.GetUserName())
	case *chatpb.Disconnect:
		sender := ctx.RemoteSender()

		if _, ok := s.clients.Load(sender.String()); !ok {
			s.logger.Warnf("cannot disconnect unknown client=(%s)", sender.String())
			return
		}

		value, ok := s.users.Load(sender.String())
		if !ok {
			s.logger.Warnf("cannot disconnect unknown user=(%s)", sender.String())
			return
		}

		userName := value.(string)
		s.clients.Delete(sender.String())
		s.users.Delete(sender.String())

		s.logger.Infof("Client=[sender=(%s), username=(%s)] disconnected", sender.String(), userName)
	case *chatpb.Message:
		sender := ctx.RemoteSender()
		s.clients.Range(func(key, value interface{}) bool {
			addr := value.(*address.Address)
			if key != sender.String() {
				ctx.RemoteForward(addr)
			}
			return true
		})
	default:
		ctx.Unhandled()
	}
}

func (s *Server) PostStop(*actors.Context) error {
	s.users.Clear()
	s.clients.Clear()
	s.logger.Info("Chat Server successfully stopped")
	return nil
}
