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

package agents

import (
	"context"
	"fmt"

	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/eventstream"
	"github.com/tochemey/goakt/v4/extension"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/llm"
)

// ADKExtensionID is the key used when registering ADKExtension with GoAkt.
const ADKExtensionID = "ADKRuntime"

// DefaultUserID is the user identity used when anonymous HTTP callers drive
// the system. Exported so the service and grain paths bind the same user ID
// to their ADK runners.
const DefaultUserID = "anonymous"

// ADKExtension carries the shared ADK runtime handles that every actor/grain
// needs at activation. The underlying model.LLM and session.Service are
// designed to be safe for concurrent use, so they are built once at process
// startup and reused across actors.
type ADKExtension struct {
	AppName        string
	LLMConfig      *llm.Config
	Model          model.LLM
	SessionService session.Service
	EventStream    eventstream.Stream
}

var _ extension.Extension = (*ADKExtension)(nil)

// ID implements extension.Extension.
func (adkExtension *ADKExtension) ID() string { return ADKExtensionID }

// NewADKExtension builds the runtime handles used by every actor and grain.
// It picks a model.LLM backend based on the provider in the llm.Config:
// Gemini providers use adk-go's native gemini.NewModel so tool-calling works;
// every other provider is wrapped via the legacy llm.Client so the existing
// OpenAI/Anthropic/Mistral keys keep working as a single-turn fallback.
func NewADKExtension(ctx context.Context, appName string, llmConfig *llm.Config, sessionService session.Service, eventStream eventstream.Stream) (*ADKExtension, error) {
	if llmConfig == nil {
		return nil, fmt.Errorf("llm config is required")
	}

	if sessionService == nil {
		return nil, fmt.Errorf("session service is required")
	}

	llmModel, err := llm.NewModelForConfig(ctx, llmConfig)
	if err != nil {
		return nil, fmt.Errorf("build ADK model: %w", err)
	}

	return &ADKExtension{
		AppName:        appName,
		LLMConfig:      llmConfig,
		Model:          llmModel,
		SessionService: sessionService,
		EventStream:    eventStream,
	}, nil
}

// FindADKExtension walks the ActorSystem's Extensions slice looking for the
// shared ADK runtime. Returns nil if the extension was not registered at
// bootstrap — callers should surface that as an activation error.
func FindADKExtension(actorSystem goakt.ActorSystem) *ADKExtension {
	for _, ext := range actorSystem.Extensions() {
		if ext == nil {
			continue
		}

		if adkExtension, ok := ext.(*ADKExtension); ok {
			return adkExtension
		}
	}
	return nil
}
