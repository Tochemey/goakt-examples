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

// Package llm provides minimal single-turn HTTP clients for OpenAI,
// Anthropic, Google, and Mistral, plus a Config loader that reads
// provider selection and API keys from the environment.
//
// The public surface is the Client interface (single Complete method)
// and the Config struct. After the ADK refactor these clients are
// consumed indirectly: actors/llm_adapter.go wraps a Client as an adk-go
// model.LLM for every provider except Gemini, which goes through
// adk-go's native gemini.NewModel so tool calling works end-to-end.
//
// ConfigExtension remains as a GoAkt extension type for backwards
// compatibility with callers that have not migrated to ADKExtension.
package llm
