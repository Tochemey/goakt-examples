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

const summarizerSystemPrompt = "You are a summarization assistant. Summarize the given content concisely while preserving key information."

// SummarizerAgent summarizes long text or content
type SummarizerAgent struct {
	baseAgent
}

var _ goakt.Actor = (*SummarizerAgent)(nil)

// NewSummarizerAgent creates a new summarizer agent
func NewSummarizerAgent() *SummarizerAgent {
	return &SummarizerAgent{}
}

// PreStart initializes the LLM client
func (s *SummarizerAgent) PreStart(ctx *goakt.Context) error {
	s.initLLMClient(ctx)
	return nil
}

// PostStop is a no-op
func (s *SummarizerAgent) PostStop(ctx *goakt.Context) error {
	return nil
}

// Receive handles summarization tasks
func (s *SummarizerAgent) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ProcessQuery:
		s.handleProcessQuery(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (s *SummarizerAgent) handleProcessQuery(ctx *goakt.ReceiveContext, msg *messages.ProcessQuery) {
	prompt := "Summarize the following:\n\n" + msg.Query
	if msg.Context != "" {
		prompt = "Context: " + msg.Context + "\n\nContent to summarize:\n" + msg.Query
	}
	result, err := s.completeWithLLM(ctx, prompt, summarizerSystemPrompt)
	if err != nil {
		s.respondError(ctx, msg.TaskID, err.Error())
		return
	}
	s.respondResult(ctx, msg.TaskID, result)
}
