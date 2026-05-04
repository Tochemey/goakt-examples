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
	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
)

const researchSystemPrompt = "You are a research assistant. Provide factual, well-researched information. Be concise and accurate."

// ResearchAgent performs research using LLM APIs
type ResearchAgent struct {
	baseAgent
}

var _ goakt.Actor = (*ResearchAgent)(nil)

// NewResearchAgent creates a new research agent
func NewResearchAgent() *ResearchAgent {
	return &ResearchAgent{}
}

// PreStart initializes the LLM client
func (r *ResearchAgent) PreStart(ctx *goakt.Context) error {
	r.initLLMClient(ctx)
	return nil
}

// PostStop is a no-op
func (r *ResearchAgent) PostStop(ctx *goakt.Context) error {
	return nil
}

// Receive handles research tasks
func (r *ResearchAgent) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ProcessQuery:
		r.handleProcessQuery(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (r *ResearchAgent) handleProcessQuery(ctx *goakt.ReceiveContext, msg *messages.ProcessQuery) {
	prompt := msg.Query
	if msg.Context != "" {
		prompt = "Context: " + msg.Context + "\n\nQuery: " + msg.Query
	}
	result, err := r.completeWithLLM(ctx, prompt, researchSystemPrompt)
	if err != nil {
		r.respondError(ctx, msg.TaskID, err.Error())
		return
	}
	r.respondResult(ctx, msg.TaskID, result)
}
