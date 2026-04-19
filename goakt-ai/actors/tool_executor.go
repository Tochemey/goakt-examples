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
	"fmt"

	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/agents"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/telemetry"
)

// ToolExecutorRouter is the actor name that should be assigned to the router
// when SpawnRouter is called from cmd/run.go. Callers send
// *messages.ExecuteTool to this name via ctx.SendAsync / ctx.SendSync and
// the configured routing strategy picks a routee.
const ToolExecutorRouter = "tool-executor-router"

// ToolExecutor is the pooled routee behind the tool-execution router. It
// carries out the same deterministic math the ADK tool sub-agent performs —
// but as a routable actor so multiple tool calls can fan out in parallel
// across the pool (and, in a cluster, across nodes). The ADK tool sub-agent
// stays the primary tool path; this router exists so that external callers
// who still speak the legacy *messages.ExecuteTool contract keep a
// concurrent execution path.
type ToolExecutor struct{}

var _ goakt.Actor = (*ToolExecutor)(nil)

// NewToolExecutor returns a freshly initialised routee.
func NewToolExecutor() *ToolExecutor { return &ToolExecutor{} }

// PreStart is a no-op: ToolExecutor is stateless.
func (executor *ToolExecutor) PreStart(_ *goakt.Context) error { return nil }

// PostStop is a no-op.
func (executor *ToolExecutor) PostStop(_ *goakt.Context) error { return nil }

// Receive dispatches tool execution requests. Unknown messages are marked
// unhandled so the router's dead-letter path can log them.
func (executor *ToolExecutor) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ExecuteTool:
		executor.handleExecute(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (executor *ToolExecutor) handleExecute(ctx *goakt.ReceiveContext, msg *messages.ExecuteTool) {
	// Publish a ToolCalled telemetry event so observers see router traffic
	// independently of whether the request came from ADK or a legacy caller.
	if extension := agents.FindADKExtension(ctx.ActorSystem()); extension != nil && extension.EventStream != nil {
		extension.EventStream.Publish(telemetry.TopicToolCalled, telemetry.ToolCalledEvent{
			Tool:      msg.Tool,
			SessionID: msg.SessionID,
			Worker:    ctx.Self().Name(),
		})
	}

	var result string
	var err error

	switch msg.Tool {
	case agents.ToolNameArithmetic:
		result, err = agents.RunArithmetic(msg.A, msg.Op, msg.B)
	case agents.ToolNamePercentOf:
		result, err = agents.RunPercent(msg.Percent, msg.Value)
	default:
		err = fmt.Errorf("unknown tool: %q", msg.Tool)
	}

	if err != nil {
		ctx.Response(&messages.ToolResult{CallID: msg.CallID, Err: err.Error()})
		return
	}

	ctx.Response(&messages.ToolResult{CallID: msg.CallID, Result: result})
}
