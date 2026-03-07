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

	"github.com/tochemey/goakt-examples/v2/goakt-ai/llm"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
)

// baseAgent provides common LLM client access for specialized agents.
// The LLM client is initialized in PreStart for efficiency and GC friendliness.
type baseAgent struct {
	llmClient llm.Client
}

// initLLMClient initializes the LLM client from the extension. Call from PreStart.
func (b *baseAgent) initLLMClient(ctx *goakt.Context) {
	ext := ctx.Extension(llm.LLMConfigExtensionID)
	if ext == nil {
		ctx.Logger().Warn("LLM extension not registered")
		return
	}
	cfgExt, ok := ext.(*llm.ConfigExtension)
	if !ok || cfgExt == nil || cfgExt.Config == nil {
		ctx.Logger().Warn("LLM extension has no config")
		return
	}
	client, err := llm.NewClient(cfgExt.Config)
	if err != nil {
		ctx.Logger().Errorf("failed to create LLM client: %v", err)
		return
	}
	ctx.Logger().Infof("LLM client initialized (provider=%s, model=%s)", cfgExt.Config.Provider, cfgExt.Config.Model)
	b.llmClient = client
}

func (b *baseAgent) completeWithLLM(ctx *goakt.ReceiveContext, prompt, systemPrompt string) (string, error) {
	if b.llmClient == nil {
		return "[LLM not configured - set API key and LLM_PROVIDER]", nil
	}
	return b.llmClient.Complete(ctx.Context(), prompt, systemPrompt)
}

func (b *baseAgent) respondResult(ctx *goakt.ReceiveContext, taskID, result string) {
	ctx.Response(&messages.QueryResult{TaskID: taskID, Result: result})
}

func (b *baseAgent) respondError(ctx *goakt.ReceiveContext, taskID, errMsg string) {
	ctx.Response(&messages.QueryResult{TaskID: taskID, Err: errMsg})
}
