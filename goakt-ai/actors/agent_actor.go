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
	"context"
	"fmt"

	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/agents"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/telemetry"
)

// AgentActor is a single parametric cluster-kind that covers every
// specialized role (research, summarizer, tool).
//
// Each spawned instance is bound to one Role and hosts a dedicated
// single-role ADK runner. The actor name (passed at spawn time) is
// also the Role string, so kinds can coexist in the cluster and the
// existing routing logic (Ask-by-kind) keeps working unchanged.
//
// The actor uses a Behaviors state machine (idle → thinking → idle) with
// ADK work offloaded via PipeTo(ctx.Self(), task). Offloading is what makes
// the state machine actually fire: if we ran the ADK turn inline, the
// mailbox would stay blocked for the whole turn and no second message
// could ever observe the thinking behavior. With PipeTo, the original
// message returns immediately, new ProcessQuery arrivals are Stashed by
// the thinking behavior, and the taskComplete pipe reply (routed to self)
// triggers Unstash + UnBecome. That is the exact pattern GoAkt's Become
// + Stash were designed for.
type AgentActor struct {
	baseAgent
	role agents.Role
}

var _ goakt.Actor = (*AgentActor)(nil)

// taskComplete is the message AgentActor sends to itself via PipeTo when
// an ADK turn finishes. It carries the original caller's PID so the
// completion handler can reply to the right actor, and the TaskID so the
// reply matches the original ProcessQuery.
type taskComplete struct {
	taskID string
	sender *goakt.PID
	result string
	err    string
}

// NewAgentActor returns a fresh AgentActor for role. The role is what the
// actor system uses to look up the sub-agent implementation at PreStart.
func NewAgentActor(role agents.Role) *AgentActor {
	return &AgentActor{role: role}
}

// PreStart resolves the ADK extension and builds the runner for this role.
func (actor *AgentActor) PreStart(ctx *goakt.Context) error {
	if actor.role == "" {
		return fmt.Errorf("AgentActor: role is required")
	}

	extension := ctx.Extension(agents.ADKExtensionID)
	if extension == nil {
		return fmt.Errorf("ADK extension not registered")
	}

	adkExtension, ok := extension.(*agents.ADKExtension)
	if !ok || adkExtension == nil {
		return fmt.Errorf("invalid ADK extension type: %T", extension)
	}

	roleAgent, err := agents.BuildSingleRoleAgent(actor.role, adkExtension.Model)
	if err != nil {
		return fmt.Errorf("build role agent: %w", err)
	}

	return actor.initADKFromContext(ctx, roleAgent)
}

// PostStop is a no-op; the ADK runner holds no resources beyond the model
// and session.Service, which are owned by the extension.
func (actor *AgentActor) PostStop(_ *goakt.Context) error { return nil }

// Receive is the default (idle) behavior. ProcessQuery triggers an async
// ADK turn via PipeTo; the actor immediately becomes thinking and returns
// from Receive, freeing the mailbox. taskComplete arriving here means an
// orphan reply (e.g. UnBecome was called too early); it is logged and
// dropped via Unhandled.
func (actor *AgentActor) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ProcessQuery:
		actor.beginTurn(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

// thinking is pushed onto the behavior stack while an ADK turn is in
// flight. New ProcessQuery messages that arrive during this window are
// stashed; the taskComplete reply pops the stack and replays them in
// arrival order under idle.
func (actor *AgentActor) thinking(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ProcessQuery:
		ctx.Stash()
	case *taskComplete:
		actor.finishTurn(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

// beginTurn kicks off one ADK turn in a pipe-managed goroutine. The
// closure captures only value types (msg, sender PID, prompt, role) so it
// can outlive the current ReceiveContext safely. The task ctx is
// intentionally context.Background() because the actor's receive context
// is cancelled when Receive returns — using it would cancel the ADK run
// immediately.
func (actor *AgentActor) beginTurn(ctx *goakt.ReceiveContext, msg *messages.ProcessQuery) {
	sender := ctx.Sender()
	prompt := buildRolePrompt(actor.role, msg)

	ctx.BecomeStacked(actor.thinking)
	ctx.PipeTo(ctx.Self(), func() (any, error) {
		result, runErr := actor.runTurnCollecting(context.Background(), msg.TaskID, agents.DefaultUserID, prompt)
		completion := &taskComplete{taskID: msg.TaskID, sender: sender}
		if runErr != nil {
			completion.err = runErr.Error()
		} else {
			completion.result = result
		}
		// Always return nil so the pipe delivers the completion message
		// rather than routing the task error to deadletter; our own
		// taskComplete.err carries the error back to the caller.
		return completion, nil
	})
}

// finishTurn replies to the original sender, publishes telemetry, and
// returns the actor to idle. Called only under the thinking behavior.
func (actor *AgentActor) finishTurn(ctx *goakt.ReceiveContext, completion *taskComplete) {
	if completion.sender != nil && completion.sender != ctx.ActorSystem().NoSender() {
		reply := &messages.QueryResult{TaskID: completion.taskID}
		if completion.err != "" {
			reply.Err = completion.err
		} else {
			reply.Result = completion.result
		}
		ctx.Tell(completion.sender, reply)
	}

	if completion.err != "" {
		actor.publishErrorEvent(completion.taskID, completion.err)
	} else {
		actor.publishTurnEvent(completion.taskID, completion.result)
	}

	ctx.UnstashAll()
	ctx.UnBecome()
}

// buildRolePrompt applies the same context/prefix composition the legacy
// per-role agents used. Kept as a free function because beginTurn needs
// to capture a plain string into the pipe closure — calling a method on
// the actor from the closure would pin the actor pointer across the
// goroutine boundary for no benefit.
func buildRolePrompt(role agents.Role, msg *messages.ProcessQuery) string {
	switch role {
	case agents.RoleResearch:
		if msg.Context != "" {
			return "Context: " + msg.Context + "\n\nQuery: " + msg.Query
		}
		return msg.Query
	case agents.RoleSummarizer:
		if msg.Context != "" {
			return "Context: " + msg.Context + "\n\nContent to summarize:\n" + msg.Query
		}
		return "Summarize the following:\n\n" + msg.Query
	default:
		return msg.Query
	}
}

// publishTurnEvent broadcasts a TurnFinished event on the shared eventstream
// so TelemetryActor (or any other subscriber) can observe per-role activity.
func (actor *AgentActor) publishTurnEvent(taskID, result string) {
	if actor.extension == nil || actor.extension.EventStream == nil {
		return
	}

	actor.extension.EventStream.Publish(telemetry.TopicTurnFinished, telemetry.TurnFinishedEvent{
		Role:   string(actor.role),
		TaskID: taskID,
		Chars:  len(result),
	})
}

// publishErrorEvent broadcasts the error path of a turn for observability.
func (actor *AgentActor) publishErrorEvent(taskID, errorMessage string) {
	if actor.extension == nil || actor.extension.EventStream == nil {
		return
	}

	actor.extension.EventStream.Publish(telemetry.TopicLLMError, telemetry.LLMErrorEvent{
		Role:   string(actor.role),
		TaskID: taskID,
		Error:  errorMessage,
	})
}
