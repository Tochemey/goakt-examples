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
	"os"
	"strings"
)

// ConfigFromEnv loads LLM config from environment variables
func ConfigFromEnv() *Config {
	provider := Provider(strings.ToLower(os.Getenv("LLM_PROVIDER")))
	if provider == "" {
		provider = ProviderOpenAI
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		switch provider {
		case ProviderOpenAI:
			model = "gpt-4o-mini"
		case ProviderAnthropic:
			model = "claude-3-5-haiku-20241022"
		case ProviderGoogle:
			model = "gemini-2.5-flash-lite"
		case ProviderMistral:
			model = "mistral-small-latest"
		default:
			model = "gpt-4o-mini"
		}
	}
	return &Config{
		Provider:     provider,
		OpenAIKey:    os.Getenv("OPENAI_API_KEY"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		GoogleKey:    os.Getenv("GOOGLE_API_KEY"),
		MistralKey:   os.Getenv("MISTRAL_API_KEY"),
		Model:        model,
		MaxTokens:    1024,
	}
}
