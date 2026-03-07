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

import "fmt"

// NewClient creates an LLM client for the given provider
func NewClient(cfg *Config) (Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	switch cfg.Provider {
	case ProviderOpenAI:
		if cfg.OpenAIKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is required for OpenAI")
		}
		return NewOpenAIClient(cfg.OpenAIKey, cfg.Model, cfg.MaxTokens), nil
	case ProviderAnthropic:
		if cfg.AnthropicKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required for Anthropic")
		}
		return NewAnthropicClient(cfg.AnthropicKey, cfg.Model, cfg.MaxTokens), nil
	case ProviderGoogle:
		if cfg.GoogleKey == "" {
			return nil, fmt.Errorf("GOOGLE_API_KEY is required for Google")
		}
		return NewGoogleClient(cfg.GoogleKey, cfg.Model, cfg.MaxTokens), nil
	case ProviderMistral:
		if cfg.MistralKey == "" {
			return nil, fmt.Errorf("MISTRAL_API_KEY is required for Mistral")
		}
		return NewMistralClient(cfg.MistralKey, cfg.Model, cfg.MaxTokens), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
