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
	"regexp"
	"strconv"
	"strings"

	goakt "github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
)

const toolSystemPrompt = "You are a tool assistant. For math calculations, extract the expression and respond with ONLY the numeric result. For other tasks, be concise."

var (
	percentRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%\s*of\s*(\d+(?:\.\d+)?)`)
	arithRe   = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*([+\-*/])\s*(\d+(?:\.\d+)?)`)
)

// ToolAgent executes tools (calculator, etc.)
type ToolAgent struct {
	baseAgent
}

var _ goakt.Actor = (*ToolAgent)(nil)

// NewToolAgent creates a new tool agent
func NewToolAgent() *ToolAgent {
	return &ToolAgent{}
}

// PreStart initializes the LLM client
func (t *ToolAgent) PreStart(ctx *goakt.Context) error {
	t.initLLMClient(ctx)
	return nil
}

// PostStop is a no-op
func (t *ToolAgent) PostStop(ctx *goakt.Context) error {
	return nil
}

// Receive handles tool execution tasks
func (t *ToolAgent) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *messages.ProcessQuery:
		t.handleProcessQuery(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (t *ToolAgent) handleProcessQuery(ctx *goakt.ReceiveContext, msg *messages.ProcessQuery) {
	// Try simple math first (e.g., "15% of 1000", "2+2", "100 * 0.15")
	if result, ok := t.tryMath(msg.Query); ok {
		t.respondResult(ctx, msg.TaskID, result)
		return
	}
	// Fall back to LLM for complex tool use
	result, err := t.completeWithLLM(ctx, msg.Query, toolSystemPrompt)
	if err != nil {
		t.respondError(ctx, msg.TaskID, err.Error())
		return
	}
	t.respondResult(ctx, msg.TaskID, result)
}

func (t *ToolAgent) tryMath(query string) (string, bool) {
	query = strings.TrimSpace(strings.ToLower(query))
	if m := percentRe.FindStringSubmatch(query); len(m) == 3 {
		pct, _ := strconv.ParseFloat(m[1], 64)
		val, _ := strconv.ParseFloat(m[2], 64)
		result := val * (pct / 100)
		return fmt.Sprintf("%.2f", result), true
	}
	if m := arithRe.FindStringSubmatch(query); len(m) == 4 {
		a, _ := strconv.ParseFloat(m[1], 64)
		b, _ := strconv.ParseFloat(m[3], 64)
		var r float64
		switch m[2] {
		case "+":
			r = a + b
		case "-":
			r = a - b
		case "*":
			r = a * b
		case "/":
			if b == 0 {
				return "division by zero", true
			}
			r = a / b
		default:
			return "", false
		}
		return fmt.Sprintf("%.2f", r), true
	}
	return "", false
}
