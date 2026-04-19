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
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
)

// Role enumerates the specialized agents the original example exposed as
// cluster kinds. The string values are preserved so existing
// `messages.ProcessQuery.TaskType` payloads from any other client keep working.
type Role string

const (
	RoleResearch    Role = "ResearchAgent"
	RoleSummarizer  Role = "SummarizerAgent"
	RoleTool        Role = "ToolAgent"
	RoleOrchestrate Role = "OrchestratorAgent"
)

// Names the ADK LlmAgent constructors register for each sub-agent. They
// double as the Author field on session.Event records, so keeping them
// stable is part of the persistence contract when the database session
// service is enabled.
const (
	AgentNameOrchestrator = "orchestrator"
	AgentNameResearch     = "research_agent"
	AgentNameSummarizer   = "summarizer_agent"
	AgentNameTool         = "tool_agent"
)

// System prompts preserved verbatim from the legacy actors package so agent
// behavior does not drift during the migration.
const (
	orchestratorInstruction = "You are an AI assistant. Answer the user's question concisely and helpfully. " +
		"Delegate to the research, summarize, or tool sub-agents when appropriate."
	researchInstruction   = "You are a research assistant. Provide factual, well-researched information. Be concise and accurate."
	summarizerInstruction = "You are a summarization assistant. Summarize the given content concisely while preserving key information."
	toolInstruction       = "You are a tool assistant. For math calculations, call the arithmetic or percent_of tool. For other tasks, be concise."
)

// BuildSingleRoleAgent constructs an LlmAgent for one role. Used by
// AgentActor cluster kinds, which each host exactly one role so the cluster
// can distribute LLM work across nodes just like the legacy Research/
// Summarizer/Tool actors did.
func BuildSingleRoleAgent(role Role, llmModel model.LLM) (agent.Agent, error) {
	switch role {
	case RoleResearch:
		return llmagent.New(llmagent.Config{
			Name:        AgentNameResearch,
			Model:       llmModel,
			Description: "Answers factual research questions.",
			Instruction: researchInstruction,
		})
	case RoleSummarizer:
		return llmagent.New(llmagent.Config{
			Name:        AgentNameSummarizer,
			Model:       llmModel,
			Description: "Summarizes long text while preserving key information.",
			Instruction: summarizerInstruction,
		})
	case RoleTool:
		tools, err := builtinTools()
		if err != nil {
			return nil, err
		}
		return llmagent.New(llmagent.Config{
			Name:        AgentNameTool,
			Model:       llmModel,
			Description: "Runs deterministic tools (arithmetic, percent_of) and falls back to a concise LLM answer.",
			Instruction: toolInstruction,
			Tools:       tools,
		})
	default:
		return nil, fmt.Errorf("unknown role: %q", role)
	}
}

// BuildRootAgentForStreaming is the exported door the service package uses
// when constructing a per-request ADK runner for SSE streaming. It builds a
// fresh root agent tree against the supplied model. Returning a new agent
// per call is cheap and keeps the streaming runner from accidentally sharing
// any call-site state.
func BuildRootAgentForStreaming(llmModel model.LLM) (agent.Agent, error) {
	if llmModel == nil {
		return nil, fmt.Errorf("model is required")
	}
	return BuildRootAgent(llmModel)
}

// BuildRootAgent wires the three specialized agents as tools underneath a
// root orchestrator LlmAgent. This replaces the regex routing in the legacy
// actors/orchestrator.go: the LLM itself now decides which sub-agent runs
// for any given user query, using each sub-agent's Description.
func BuildRootAgent(llmModel model.LLM) (agent.Agent, error) {
	researchAgent, err := BuildSingleRoleAgent(RoleResearch, llmModel)
	if err != nil {
		return nil, fmt.Errorf("build research sub-agent: %w", err)
	}

	summarizerAgent, err := BuildSingleRoleAgent(RoleSummarizer, llmModel)
	if err != nil {
		return nil, fmt.Errorf("build summarizer sub-agent: %w", err)
	}

	toolAgent, err := BuildSingleRoleAgent(RoleTool, llmModel)
	if err != nil {
		return nil, fmt.Errorf("build tool sub-agent: %w", err)
	}

	return llmagent.New(llmagent.Config{
		Name:        AgentNameOrchestrator,
		Model:       llmModel,
		Description: "Routes user questions to research, summarizer, or tool sub-agents.",
		Instruction: orchestratorInstruction,
		Tools: []tool.Tool{
			agenttool.New(researchAgent, nil),
			agenttool.New(summarizerAgent, nil),
			agenttool.New(toolAgent, nil),
		},
	})
}
