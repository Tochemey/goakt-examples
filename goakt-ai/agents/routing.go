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

import "strings"

// RouteByKeyword is the fallback router used by ConversationGrain when the
// configured LLM provider cannot do function calling (the llm adapter
// strips tools) — in that case the root orchestrator LlmAgent cannot
// delegate to its sub-agent tools, so we pre-select the role here and
// drive a single-role runner directly.
//
// The match order and rules are intentionally identical to the legacy
// implementation so behavior stays stable for non-Gemini users:
//   - "X% of Y" -> tool
//   - contains any of +, -, *, / -> tool
//   - contains "summar" -> summarizer
//   - otherwise -> research
func RouteByKeyword(query string) Role {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))

	if strings.Contains(normalizedQuery, "%") && strings.Contains(normalizedQuery, "of") {
		return RoleTool
	}

	if strings.ContainsAny(normalizedQuery, "+-*/") {
		return RoleTool
	}

	if strings.Contains(normalizedQuery, "summar") {
		return RoleSummarizer
	}

	return RoleResearch
}

// PrefixPromptForRole rebuilds the prompt prefixes the legacy per-role
// actors used when Context was supplied on ProcessQuery. The root ADK
// orchestrator does not see these prefixes because it runs whole-query
// through its Instruction; the fallback path does, because it drives a
// single-role runner whose Instruction is role-specific but otherwise
// generic.
func PrefixPromptForRole(role Role, query, promptContext string) string {
	switch role {
	case RoleResearch:
		if promptContext != "" {
			return "Context: " + promptContext + "\n\nQuery: " + query
		}
		return query
	case RoleSummarizer:
		if promptContext != "" {
			return "Context: " + promptContext + "\n\nContent to summarize:\n" + query
		}
		return "Summarize the following:\n\n" + query
	default:
		return query
	}
}
