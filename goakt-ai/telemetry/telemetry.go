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

package telemetry

import (
	"github.com/tochemey/goakt/v4/eventstream"
	"github.com/tochemey/goakt/v4/log"
)

// Eventstream topics published by the ADK integration. Any actor that wants
// to observe agent activity subscribes to these via the shared bus held on
// ADKExtension. The names are stable strings so external tooling (logs,
// dashboards) can match on them.
const (
	TopicTurnFinished    = "ai.turn.finished"
	TopicLLMError        = "ai.llm.error"
	TopicGrainPassivated = "ai.grain.passivated"
	TopicToolCalled      = "ai.tool.called"
)

// RoleOrchestrator is the Role field value for events emitted from the
// ConversationGrain orchestrator path. AgentActor instances use their own
// role string directly, but the grain's root runner is not tied to a single
// Role, so it publishes under this fixed string.
const RoleOrchestrator = "orchestrator"

// TurnFinishedEvent is published after an ADK turn completes successfully.
type TurnFinishedEvent struct {
	Role   string
	TaskID string
	Chars  int
}

// LLMErrorEvent is published when the LLM or runner returns an error.
type LLMErrorEvent struct {
	Role   string
	TaskID string
	Error  string
}

// GrainPassivatedEvent is published when a ConversationGrain is deactivated
// due to idle timeout. Useful for capacity dashboards.
type GrainPassivatedEvent struct {
	SessionID string
}

// ToolCalledEvent is published when a tool is dispatched through the
// ToolExecutor router (observability on fan-out traffic).
type ToolCalledEvent struct {
	Tool      string
	SessionID string
	Worker    string
}

// StartTelemetryLogger subscribes to the topics above and logs every event
// at Info level. The returned subscriber is owned by the caller; it must
// unsubscribe (via bus.RemoveSubscriber) on shutdown to stop the goroutine.
//
// This is a standalone subscriber (not an actor) because eventstream's
// Subscriber API pushes messages on a channel — the simplest consumer is a
// lightweight goroutine, not a full actor mailbox.
func StartTelemetryLogger(eventStream eventstream.Stream, logger log.Logger) eventstream.Subscriber {
	subscriber := eventStream.AddSubscriber()
	topics := []string{TopicTurnFinished, TopicLLMError, TopicGrainPassivated, TopicToolCalled}

	for _, topic := range topics {
		eventStream.Subscribe(subscriber, topic)
	}

	go func() {
		for event := range subscriber.Iterator() {
			if event == nil {
				return
			}
			logger.Infof("telemetry: topic=%s payload=%#v", event.Topic(), event.Payload())
		}
	}()

	return subscriber
}

// StartDeadLetterLogger subscribes to the actor system's built-in events
// topic (which carries Deadletter, ActorStopped, and similar lifecycle
// events) and logs every dropped/stopped signal. Uses ActorSystem.Subscribe
// so the returned subscriber is bound to the system's shared events stream,
// not the application bus.
//
// The returned function unsubscribes the caller on shutdown; not calling it
// leaks a goroutine and a subscriber slot.
func StartDeadLetterLogger(subscriber eventstream.Subscriber, logger log.Logger) {
	go func() {
		for event := range subscriber.Iterator() {
			if event == nil {
				return
			}

			// The events topic carries a mix of actor lifecycle messages;
			// we log every one at Warn level so deadletters are visible,
			// while leaving classification to the reader.
			logger.Warnf("actor-system-event: topic=%s payload=%#v", event.Topic(), event.Payload())
		}
	}()
}
