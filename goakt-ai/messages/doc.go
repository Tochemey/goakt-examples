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

// Package messages defines the plain-Go message types exchanged between
// actors, grains, and the HTTP layer.
//
// The shapes divide into three groups:
//
//   - Request/response:   SubmitQuery, QueryResponse (HTTP ↔ grain);
//     ProcessQuery, QueryResult (legacy callers ↔ AgentActor).
//
//   - Tool dispatch:      ExecuteTool, ToolResult (router path for
//     deterministic arithmetic / percent_of).
//
//   - Streaming:          StreamToken, emitted by the SSE producer and
//     piped through a goakt/v4/stream pipeline.
//
// Every type intended to cross a cluster boundary is registered with
// remote.WithSerializables at actor-system bootstrap; the stream-only
// types are deliberately in-process.
package messages
