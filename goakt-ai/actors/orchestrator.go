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
	"strings"
	"time"

	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
)

const (
	kindResearch    = "ResearchAgent"
	kindSummarizer  = "SummarizerAgent"
	kindTool        = "ToolAgent"
	askTimeout      = 60 * time.Second
	orchestratorSys = "You are an AI assistant. Answer the user's question concisely and helpfully."
)

// OrchestratorAgent coordinates queries and delegates to specialized agents
type OrchestratorAgent struct {
	baseAgent
}

var _ goakt.Actor = (*OrchestratorAgent)(nil)

// NewOrchestratorAgent creates a new orchestrator agent
func NewOrchestratorAgent() *OrchestratorAgent {
	return &OrchestratorAgent{}
}

// PreStart initializes the LLM client for fallback when agents are unavailable
func (o *OrchestratorAgent) PreStart(ctx *goakt.Context) error {
	o.initLLMClient(ctx)
	return nil
}

// PostStop is a no-op
func (o *OrchestratorAgent) PostStop(ctx *goakt.Context) error {
	return nil
}

// Receive handles query submission and delegation
func (o *OrchestratorAgent) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.SubmitQuery:
		o.handleSubmitQuery(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (o *OrchestratorAgent) handleSubmitQuery(ctx *goakt.ReceiveContext, msg *messages.SubmitQuery) {
	// Determine which agent to use
	kind := o.selectAgentKind(msg.Query)
	pid, err := o.getOrSpawnAgent(ctx, kind)
	if err != nil {
		// Fallback to LLM directly if we can't get an agent
		result, errResp := o.completeWithLLM(ctx, msg.Query, orchestratorSys)
		if errResp != nil {
			ctx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Err: errResp.Error()})
			return
		}
		ctx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Result: result})
		return
	}

	// Delegate to specialized agent (retry once on failure for transient relocation)
	taskID := msg.SessionID + "-task"
	req := &messages.ProcessQuery{TaskID: taskID, Query: msg.Query, TaskType: kind}
	reply, err := goakt.Ask(ctx.Context(), pid, req, askTimeout)
	if err != nil {
		// Retry once with a fresh agent reference (handles actor mid-relocation or host crash)
		pid2, err2 := o.getOrSpawnAgent(ctx, kind)
		if err2 == nil {
			reply, err = goakt.Ask(ctx.Context(), pid2, req, askTimeout)
		}
		if err != nil {
			ctx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Err: err.Error()})
			return
		}
	}

	qr, ok := reply.(*messages.QueryResult)
	if !ok {
		ctx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Err: "invalid response type"})
		return
	}
	if qr.Err != "" {
		ctx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Err: qr.Err})
		return
	}
	ctx.Response(&messages.QueryResponse{SessionID: msg.SessionID, Result: qr.Result})
}

func (o *OrchestratorAgent) getOrSpawnAgent(ctx *goakt.ReceiveContext, kind string) (*goakt.PID, error) {
	system := ctx.ActorSystem()
	pid, err := system.ActorOf(ctx.Context(), kind)
	if err == nil {
		return pid, nil
	}
	var agent goakt.Actor
	switch kind {
	case kindResearch:
		agent = NewResearchAgent()
	case kindSummarizer:
		agent = NewSummarizerAgent()
	case kindTool:
		agent = NewToolAgent()
	default:
		agent = NewResearchAgent()
	}
	return system.Spawn(ctx.Context(), kind, agent, goakt.WithLongLived())
}

func (o *OrchestratorAgent) selectAgentKind(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	// Math: percent, arithmetic
	if strings.Contains(q, "%") && strings.Contains(q, "of") {
		return kindTool
	}
	if strings.ContainsAny(q, "+-*/") {
		for _, c := range q {
			if c == '+' || c == '-' || c == '*' || c == '/' {
				return kindTool
			}
		}
	}
	// Summarization
	if strings.Contains(q, "summar") {
		return kindSummarizer
	}
	// Default: research
	return kindResearch
}
