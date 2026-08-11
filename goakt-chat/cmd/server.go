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

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/supervisor"

	"github.com/tochemey/goakt-examples/v2/goakt-chat/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-chat/wire"
	"github.com/tochemey/goakt-examples/v2/internal/remoting"
)

var (
	serverHost string
	serverPort int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the chat server",
	Long: `Start the chat server. Clients connect to this server to chat in rooms.

The server binds to the specified host and port. Start the server before
connecting any clients. Use --host 0.0.0.0 to accept connections from other machines.

The server has no --codec flag: it registers both wire formats and replies to each
client using the format that client connected with.`,
	Example: `  chat server
  chat server --host 0.0.0.0 --port 5000`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVar(&serverHost, "host", "127.0.0.1", "Host to bind the server to")
	serverCmd.Flags().IntVar(&serverPort, "port", 4000, "Port to listen on")
}

func runServer(*cobra.Command, []string) error {
	ctx := context.Background()

	// wire.RemoteOptions is shared with the client command so both ends of the
	// connection always register the same set of types.
	actorSystem, err := actor.NewActorSystem(
		"ChatSystem",
		actor.WithRemote(remoting.NewConfig(serverHost, serverPort, wire.RemoteOptions()...)),
		actor.WithLoggingDisabled())

	if err != nil {
		return fmt.Errorf("failed to create actor system: %w", err)
	}

	if err := actorSystem.Start(ctx); err != nil {
		return fmt.Errorf("failed to start actor system: %w", err)
	}

	if _, err := actorSystem.Spawn(
		ctx,
		"ChatServer",
		actors.NewServer(),
		actor.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			))); err != nil {
		return fmt.Errorf("failed to spawn ChatServer: %w", err)
	}

	fmt.Printf("Chat Server is running on %s port %d — accepting %s and %s clients\n",
		serverHost, serverPort, wire.Proto, wire.CBOR)

	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	_ = actorSystem.Stop(ctx)
	return nil
}
