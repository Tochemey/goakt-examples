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
	adkrunner "google.golang.org/adk/runner"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/agents"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/telemetry"
)

// ConversationGrain is a session-scoped virtual actor that owns ADK runners
// for the root orchestrator tree and each specialized role. GoAkt guarantees
// a single-writer per grain identity across the cluster, so the ADK session
// history for a given SessionID stays consistent even under concurrent HTTP
// requests.
//
// Activation: the grain is created on first message; passivation is
// configured via WithGrainDeactivateAfter at registration time, so idle
// sessions free their runner and let the underlying session.Service shed
// memory.
//
// Routing: the legacy llm.Client adapter strips tool declarations, so the
// root orchestrator cannot delegate via function calls. roleRunners holds
// one runner per role; handleSubmit uses agents.RouteByKeyword to pick
// which one to run, preserving the legacy routing behavior.
type ConversationGrain struct {
	baseAgent
	sessionID   string
	roleRunners map[agents.Role]*adkrunner.Runner
}

var _ goakt.Grain = (*ConversationGrain)(nil)

// NewConversationGrainFactory returns a GrainFactory that constructs a
// ConversationGrain per identity. It is safe to register this factory once
// at process startup; GoAkt calls it on demand for each new session.
func NewConversationGrainFactory() goakt.GrainFactory {
	return func(_ context.Context) (goakt.Grain, error) {
		return &ConversationGrain{}, nil
	}
}

// OnActivate resolves the ADK extension, builds the root agent tree, and
// wires a runner that shares the cluster-wide session.Service. The grain
// identity Name is adopted as the ADK session ID so the runner can look up
// or create the exact session that belongs to this grain.
//
// Per-role runners are also built for the non-Gemini fallback path so the
// first SubmitQuery doesn't pay build cost. Runner construction is cheap —
// it reuses the shared model.LLM and session.Service.
func (grain *ConversationGrain) OnActivate(_ context.Context, props *goakt.GrainProps) error {
	grain.sessionID = props.Identity().Name()

	actorSystem := props.ActorSystem()
	if actorSystem == nil {
		return fmt.Errorf("grain has no actor system")
	}

	extension := agents.FindADKExtension(actorSystem)
	if extension == nil {
		return fmt.Errorf("ADK extension not registered")
	}
	grain.extension = extension

	rootAgent, err := agents.BuildRootAgent(extension.Model)
	if err != nil {
		return fmt.Errorf("build root agent: %w", err)
	}

	if err := grain.initRunnerFromExtension(rootAgent); err != nil {
		return err
	}

	return grain.buildRoleRunners()
}

// buildRoleRunners constructs one runner per role, each backed by a
// single-role LlmAgent. The runners share the extension's session.Service
// so history from any role contributes to the same conversation trail.
func (grain *ConversationGrain) buildRoleRunners() error {
	grain.roleRunners = make(map[agents.Role]*adkrunner.Runner, 3)

	for _, role := range []agents.Role{agents.RoleResearch, agents.RoleSummarizer, agents.RoleTool} {
		roleAgent, err := agents.BuildSingleRoleAgent(role, grain.extension.Model)
		if err != nil {
			return fmt.Errorf("build %s agent: %w", role, err)
		}

		runner, err := adkrunner.New(adkrunner.Config{
			AppName:           grain.extension.AppName,
			Agent:             roleAgent,
			SessionService:    grain.extension.SessionService,
			AutoCreateSession: true,
		})
		if err != nil {
			return fmt.Errorf("build %s runner: %w", role, err)
		}

		grain.roleRunners[role] = runner
	}

	return nil
}

// OnDeactivate publishes a telemetry event so operators can track grain
// churn. ADK session state is held by the shared SessionService, so there
// is nothing grain-local to flush beyond the telemetry signal.
func (grain *ConversationGrain) OnDeactivate(_ context.Context, _ *goakt.GrainProps) error {
	if grain.extension != nil && grain.extension.EventStream != nil {
		grain.extension.EventStream.Publish(telemetry.TopicGrainPassivated, telemetry.GrainPassivatedEvent{SessionID: grain.sessionID})
	}
	return nil
}

// OnReceive is the grain's message handler. Mirrors the legacy orchestrator
// contract: SubmitQuery in, QueryResponse out (as Ask response).
func (grain *ConversationGrain) OnReceive(grainCtx *goakt.GrainContext) {
	switch msg := grainCtx.Message().(type) {
	case *messages.SubmitQuery:
		grain.handleSubmit(grainCtx, msg)
	default:
		grainCtx.Unhandled()
	}
}

func (grain *ConversationGrain) handleSubmit(grainCtx *goakt.GrainContext, msg *messages.SubmitQuery) {
	if grain.runner == nil {
		grainCtx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Err: "runner not initialized"})
		return
	}

	// The ADK runner and all sub-agents are intentionally run inside the
	// grain's single-writer mailbox. The runner's iterator is synchronous so
	// the grain stays single-writer for the duration of the turn: no second
	// SubmitQuery can interleave between events for the same session.
	runner, role := grain.pickRunner(msg.Query)
	prompt := msg.Query
	if role != "" {
		prompt = agents.PrefixPromptForRole(role, msg.Query, "")
	}

	result, err := grain.runTurnOn(grainCtx.Context(), runner, msg.SessionID, agents.DefaultUserID, prompt)
	if err != nil {
		grainCtx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Err: err.Error()})
		grain.publishError(msg.SessionID, err.Error())
		return
	}

	grainCtx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Result: result})
	grain.publishTurn(msg.SessionID, result)
}

// pickRunner returns the runner to use for this turn along with the role it
// represents. All providers go through the legacy llm.Client adapter which
// strips tool declarations, so the root orchestrator cannot delegate via
// function calls; we always pre-route with agents.RouteByKeyword and drive
// the matching single-role runner directly.
func (grain *ConversationGrain) pickRunner(query string) (*adkrunner.Runner, agents.Role) {
	role := agents.RouteByKeyword(query)
	if runner, ok := grain.roleRunners[role]; ok {
		return runner, role
	}

	// Fall back to the root runner if the role runner is missing for any
	// reason — better a generic answer than an error response.
	return grain.runner, ""
}

func (grain *ConversationGrain) publishTurn(sessionID, result string) {
	if grain.extension == nil || grain.extension.EventStream == nil {
		return
	}

	grain.extension.EventStream.Publish(telemetry.TopicTurnFinished, telemetry.TurnFinishedEvent{
		Role:   telemetry.RoleOrchestrator,
		TaskID: sessionID,
		Chars:  len(result),
	})
}

func (grain *ConversationGrain) publishError(sessionID, errorMessage string) {
	if grain.extension == nil || grain.extension.EventStream == nil {
		return
	}

	grain.extension.EventStream.Publish(telemetry.TopicLLMError, telemetry.LLMErrorEvent{
		Role:   telemetry.RoleOrchestrator,
		TaskID: sessionID,
		Error:  errorMessage,
	})
}
