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

package llm

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

// NewModelForConfig picks an adk-go model implementation that matches the
// provider already configured via env vars. Gemini routes through adk-go's
// native client so the LLM can emit function calls; every other provider is
// adapted via clientModel, which preserves single-turn completion behavior
// (no tool calls) using the existing Client implementations.
func NewModelForConfig(ctx context.Context, llmConfig *Config) (model.LLM, error) {
	switch llmConfig.Provider {
	case ProviderGoogle:
		if llmConfig.GoogleKey == "" {
			return nil, fmt.Errorf("GOOGLE_API_KEY is required for Gemini")
		}
		return gemini.NewModel(ctx, llmConfig.Model, &genai.ClientConfig{APIKey: llmConfig.GoogleKey})
	default:
		legacyClient, err := NewClient(llmConfig)
		if err != nil {
			return nil, err
		}
		return &clientModel{name: llmConfig.Model, client: legacyClient}, nil
	}
}

// clientModel adapts the legacy Client (OpenAI/Anthropic/Mistral) to
// adk-go's model.LLM interface. It flattens the genai.Content list into a
// prompt + system-prompt pair and calls Client.Complete. Tool / function
// calling is not supported through this adapter — if the LLMRequest carries
// tool declarations, the adapter ignores them and the LLM will answer in text.
type clientModel struct {
	name   string
	client Client
}

var _ model.LLM = (*clientModel)(nil)

func (adapter *clientModel) Name() string { return adapter.name }

func (adapter *clientModel) GenerateContent(ctx context.Context, request *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		prompt, systemPrompt := flattenContents(request.Contents)

		text, err := adapter.client.Complete(ctx, prompt, systemPrompt)
		if err != nil {
			yield(nil, err)
			return
		}

		response := &model.LLMResponse{
			Content:      genai.NewContentFromText(text, genai.RoleModel),
			ModelVersion: adapter.name,
			TurnComplete: true,
		}
		yield(response, nil)
	}
}

// flattenContents collapses a genai.Content slice into (prompt, system). The
// first non-user role (typically "system") becomes the system prompt; all
// user parts are joined with blank lines to form the prompt.
func flattenContents(contents []*genai.Content) (prompt, systemPrompt string) {
	var promptBuilder, systemBuilder strings.Builder

	for _, content := range contents {
		if content == nil {
			continue
		}

		for _, part := range content.Parts {
			if part == nil || part.Text == "" {
				continue
			}

			if content.Role == genai.RoleUser || content.Role == "" {
				if promptBuilder.Len() > 0 {
					promptBuilder.WriteString("\n\n")
				}
				promptBuilder.WriteString(part.Text)
			} else {
				if systemBuilder.Len() > 0 {
					systemBuilder.WriteString("\n")
				}
				systemBuilder.WriteString(part.Text)
			}
		}
	}

	return promptBuilder.String(), systemBuilder.String()
}
