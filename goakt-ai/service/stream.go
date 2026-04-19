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

package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/stream"
	adkagent "google.golang.org/adk/agent"
	adkrunner "google.golang.org/adk/runner"
	"google.golang.org/genai"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/agents"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
)

// streamBufferSize controls the channel depth between the ADK runner
// goroutine and the goakt stream source. Keep it small: backpressure from
// the SSE writer should slow down token emission, not build up unbounded.
const streamBufferSize = 32

// handleStreamQuery implements the GET /query/stream endpoint. It mirrors
// handleQuery (blocking JSON) but pipes partial ADK tokens through a
// goakt/v4/stream pipeline, which then writes Server-Sent Events to the
// client. The ADK session binding still goes through the shared
// SessionService so this turn participates in the same conversation as any
// prior JSON POST /query call (when the caller passes session_id).
func (queryService *QueryService) handleStreamQuery(responseWriter http.ResponseWriter, request *http.Request) {
	query := request.URL.Query().Get("q")
	if query == "" {
		http.Error(responseWriter, "q is required", http.StatusBadRequest)
		return
	}

	sessionID := request.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	extension := agents.FindADKExtension(queryService.actorSystem)
	if extension == nil {
		http.Error(responseWriter, "ADK extension not registered", http.StatusInternalServerError)
		return
	}

	flusher, ok := responseWriter.(http.Flusher)
	if !ok {
		http.Error(responseWriter, "streaming not supported by response writer", http.StatusInternalServerError)
		return
	}

	// Build a fresh per-request ADK runner against the shared session
	// service. Runners are cheap; the agent tree and session state are
	// shared, so this doesn't duplicate conversation history.
	rootAgent, err := agents.BuildRootAgentForStreaming(extension.Model)
	if err != nil {
		http.Error(responseWriter, fmt.Sprintf("build agent: %v", err), http.StatusInternalServerError)
		return
	}

	runner, err := adkrunner.New(adkrunner.Config{
		AppName:           extension.AppName,
		Agent:             rootAgent,
		SessionService:    extension.SessionService,
		AutoCreateSession: true,
	})
	if err != nil {
		http.Error(responseWriter, fmt.Sprintf("build runner: %v", err), http.StatusInternalServerError)
		return
	}

	responseWriter.Header().Set("Content-Type", "text/event-stream")
	responseWriter.Header().Set("Cache-Control", "no-cache")
	responseWriter.Header().Set("Connection", "keep-alive")

	tokens := make(chan messages.StreamToken, streamBufferSize)

	// Producer goroutine: drain the ADK event iterator into the channel.
	// Every send selects on r.Context().Done() so the goroutine exits if
	// the client disconnects or the sink dies before the LLM finishes —
	// otherwise a blocked send would keep the producer (and its ADK
	// runner) alive indefinitely.
	go func() {
		defer close(tokens)

		send := func(token messages.StreamToken) bool {
			select {
			case tokens <- token:
				return true
			case <-request.Context().Done():
				return false
			}
		}

		userMessage := genai.NewContentFromText(query, genai.RoleUser)
		for event, err := range runner.Run(request.Context(), agents.DefaultUserID, sessionID, userMessage, adkagent.RunConfig{StreamingMode: adkagent.StreamingModeSSE}) {
			if err != nil {
				send(messages.StreamToken{SessionID: sessionID, Err: err.Error(), Final: true})
				return
			}

			if event == nil || event.LLMResponse.Content == nil {
				continue
			}

			for _, part := range event.LLMResponse.Content.Parts {
				if part == nil || part.Text == "" {
					continue
				}

				if !send(messages.StreamToken{
					SessionID: sessionID,
					Text:      part.Text,
					Final:     event.IsFinalResponse(),
				}) {
					return
				}
			}
		}

		send(messages.StreamToken{SessionID: sessionID, Final: true})
	}()

	// goakt/v4/stream pipeline: FromChannel -> ForEach sink that writes SSE
	// frames. Running through the stream materializer (instead of a plain
	// for-range on the channel) wires backpressure, metrics, and the
	// shutdown/abort contract into the actor system automatically.
	source := stream.FromChannel(tokens)
	sink := stream.ForEach(func(token messages.StreamToken) {
		writeSSE(responseWriter, token)
		flusher.Flush()
	})

	graph := source.To(sink)
	streamHandle, err := graph.Run(request.Context(), queryService.actorSystem)
	if err != nil {
		queryService.logger.Errorf("run stream graph: %v", err)
		return
	}

	<-streamHandle.Done()
	if streamErr := streamHandle.Err(); streamErr != nil {
		queryService.logger.Errorf("stream finished with error: %v", streamErr)
	}
}

// writeSSE emits one Server-Sent Events frame with a JSON payload.
func writeSSE(responseWriter http.ResponseWriter, token messages.StreamToken) {
	payload, err := json.Marshal(token)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(responseWriter, "data: %s\n\n", payload)
}
