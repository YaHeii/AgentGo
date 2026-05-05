package app

import (
	"github.com/YaHeii/agentGo/internal/event"
	"github.com/YaHeii/agentGo/internal/message"
)

type Event = event.Event

type SessionReadyEvent = event.SessionReadyEvent
type ConversationHydratedEvent = event.ConversationHydratedEvent
type MessageCreatedEvent = event.MessageCreatedEvent
type MessageDeltaEvent = event.MessageDeltaEvent
type MessageCompletedEvent = event.MessageCompletedEvent
type MessageFailedEvent = event.MessageFailedEvent
type MessageCancelledEvent = event.MessageCancelledEvent

type SendMessageParams = message.SendMessageParams
type SendMessageResult = message.SendMessageResult
