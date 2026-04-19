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
	"context"
	"fmt"
	"strings"

	goakt "github.com/tochemey/goakt/v4/actor"
	adkagent "google.golang.org/adk/agent"
	adkrunner "google.golang.org/adk/runner"
	"google.golang.org/genai"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/agents"
)

// baseAgent holds the shared ADK runner + extension handles that every actor
// and grain needs. Kept intentionally small: it knows how to resolve the
// ADKExtension and how to execute exactly one turn through the runner.
type baseAgent struct {
	extension *agents.ADKExtension
	runner    *adkrunner.Runner
}

// initADKFromContext fetches the ADKExtension and builds a Runner bound to
// rootAgent. Call this from an actor's PreStart.
func (base *baseAgent) initADKFromContext(ctx *goakt.Context, rootAgent adkagent.Agent) error {
	extension := ctx.Extension(agents.ADKExtensionID)
	if extension == nil {
		return fmt.Errorf("ADK extension not registered")
	}

	adkExtension, ok := extension.(*agents.ADKExtension)
	if !ok || adkExtension == nil {
		return fmt.Errorf("invalid ADK extension type: %T", extension)
	}
	base.extension = adkExtension

	runner, err := adkrunner.New(adkrunner.Config{
		AppName:           adkExtension.AppName,
		Agent:             rootAgent,
		SessionService:    adkExtension.SessionService,
		AutoCreateSession: true,
	})
	if err != nil {
		return fmt.Errorf("build ADK runner: %w", err)
	}

	base.runner = runner
	return nil
}

// runTurnCollecting executes one ADK turn on the actor's default runner.
// Equivalent to runTurnOn(b.runner, ...); kept as a thin wrapper so actors
// that only have one runner don't need to reference it explicitly.
func (base *baseAgent) runTurnCollecting(ctx context.Context, sessionID, userID, query string) (string, error) {
	return base.runTurnOn(ctx, base.runner, sessionID, userID, query)
}

// runTurnOn executes one ADK turn against the supplied runner and returns
// the concatenated final text response. Tool / function-call intermediate
// events are consumed but only final, non-partial text parts are included
// in the result. Taking runner as a parameter lets ConversationGrain hold
// multiple runners (one per role) and route to the right one per turn,
// without swapping b.runner from under other helpers.
func (base *baseAgent) runTurnOn(ctx context.Context, runner *adkrunner.Runner, sessionID, userID, query string) (string, error) {
	if runner == nil {
		return "", fmt.Errorf("ADK runner not initialized")
	}

	if userID == "" {
		userID = agents.DefaultUserID
	}

	var responseBuilder strings.Builder
	userMessage := genai.NewContentFromText(query, genai.RoleUser)

	for event, err := range runner.Run(ctx, userID, sessionID, userMessage, adkagent.RunConfig{StreamingMode: adkagent.StreamingModeNone}) {
		if err != nil {
			return "", err
		}

		if event == nil || event.LLMResponse.Content == nil {
			continue
		}

		if event.LLMResponse.Partial {
			continue
		}

		if !event.IsFinalResponse() {
			continue
		}

		for _, part := range event.LLMResponse.Content.Parts {
			if part != nil && part.Text != "" {
				if responseBuilder.Len() > 0 {
					responseBuilder.WriteString("\n")
				}
				responseBuilder.WriteString(part.Text)
			}
		}
	}

	return responseBuilder.String(), nil
}

// initRunnerFromExtension builds the ADK runner using the extension already
// stored on the receiver. Used from grain activation, which has an
// ADKExtension pointer but not a full Context or GrainContext.
func (base *baseAgent) initRunnerFromExtension(rootAgent adkagent.Agent) error {
	if base.extension == nil {
		return fmt.Errorf("ADK extension not initialized on baseAgent")
	}

	runner, err := adkrunner.New(adkrunner.Config{
		AppName:           base.extension.AppName,
		Agent:             rootAgent,
		SessionService:    base.extension.SessionService,
		AutoCreateSession: true,
	})
	if err != nil {
		return fmt.Errorf("build ADK runner: %w", err)
	}

	base.runner = runner
	return nil
}
