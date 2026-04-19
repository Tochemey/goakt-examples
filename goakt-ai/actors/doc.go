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

// Package actors hosts the GoAkt actor and grain definitions used by the
// goakt-ai example. Domain concerns (agent builders, roles, tools,
// routing, the shared ADKExtension runtime) live in the agents package;
// observability lives in the telemetry package; the legacy single-turn
// LLM adapter lives in the llm package. This package is intentionally
// limited to:
//
//   - baseAgent (base_agent.go): shared ADK-runner plumbing embedded by
//     both AgentActor and ConversationGrain.
//
//   - AgentActor (agent_actor.go): cluster-kind actor parametrized by
//     Role. Implements an Idle/Thinking state machine using Behaviors +
//     Stash, with the ADK turn off-loaded via PipeTo(Self) so the
//     mailbox stays responsive during the LLM call.
//
//   - ConversationGrain (conversation_grain.go): per-SessionID virtual
//     actor that owns an ADK runner bound to the root agent tree plus a
//     per-Role runner fallback used when the configured LLM provider
//     cannot do function calling.
//
//   - ToolExecutor (tool_executor.go): router routee that serves legacy
//     ExecuteTool messages with deterministic math, used by the
//     ToolExecutorRouter pool registered at bootstrap.
package actors
