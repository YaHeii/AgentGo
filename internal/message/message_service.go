package message

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/store"
)

var errTODO = errors.New("TODO: not implemented")

type MessageService struct {
	store  messageStore
	bus    bus.Bus[Event]
	events <-chan Event
}

func NewMessageService(st messageStore) *MessageService {
	b := bus.NewBus[Event](128)

	return &MessageService{
		store:  st,
		bus:    b,
		events: b.Subscribe(context.Background()),
	}
}

// TODO:Can the sessionID be passed into the ctx file? Is the ctx reset when the agent creates a single session?
func (s *MessageService) Create(ctx context.Context, sessionID string, params CreateMessageParams) (Message, error) {
	now := time.Now().UTC()
	if len(params.Parts) == 0 {
		params.Parts = []Part{{Type: PartTypeText}}
	}
	if params.Status == "" {
		params.Status = StatusComplete
	}
	if params.FinishedAt.IsZero() {
		params.FinishedAt = now
	}

	msg := Message{
		ID:        params.ID,
		SessionID: sessionID,
		Kind:      params.Kind,
		Status:    params.Status,
		CreatedAt: now,
		UpdatedAt: now,
		Flags:     params.Flags,
		Parts:     cloneParts(params.Parts),
		System:    cloneSystemPayload(params.System),
		Progress:  cloneProgressPayload(params.Progress),
	}
	if params.IsCompactSummary {
		msg.Flags.IsCompactSummary = true
	}
	if msg.ID == "" {
		msg.ID = buildMessageID(params.Kind, now)
	}

	messageJSON, err := marshalMessageJSON(msg)
	if err != nil {
		return Message{}, err
	}

	row, err := s.store.CreateMessage(ctx, store.CreateMessageParams{
		ID:               msg.ID,
		SessionID:        sessionID,
		Kind:             string(params.Kind),
		Provider:         params.Provider,
		FinishedAt:       params.FinishedAt,
		IsCompactSummary: params.IsCompactSummary || params.Flags.IsCompactSummary,
		MessageJSON:      messageJSON,
	})
	if err != nil {
		return Message{}, err
	}

	msg, err = toMessage(row)
	if err != nil {
		return Message{}, err
	}
	msg.Status = params.Status
	msg.CreatedAt = params.FinishedAt
	msg.UpdatedAt = params.FinishedAt

	s.publish(MessageCreatedEvent{Message: msg})

	return msg, nil
}

func (s *MessageService) Update(ctx context.Context, message Message) error {
	switch message.Status {
	case StatusStreaming:
		s.publish(MessageDeltaEvent{
			Message: message,
			Delta:   textContent(message.Parts),
		})
	case StatusFailed:
		s.publish(MessageFailedEvent{Message: message})
	case StatusCancelled:
		s.publish(MessageCancelledEvent{Message: message})
	default:
		s.publish(MessageCompletedEvent{Message: message})
	}

	return nil
}

func (s *MessageService) Get(_ context.Context, _ string) (Message, error) {
	return Message{}, errTODO
}

func (s *MessageService) List(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.store.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		msg, err := toMessage(row)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func (s *MessageService) ListUserMessages(_ context.Context, _ string) ([]Message, error) {
	return nil, errTODO
}

func (s *MessageService) ListAllUserMessages(_ context.Context) ([]Message, error) {
	return nil, errTODO
}

func (s *MessageService) Delete(_ context.Context, _ string) error {
	return errTODO
}

func (s *MessageService) DeleteSessionMessages(_ context.Context, _ string) error {
	return errTODO
}

func (s *MessageService) Events() <-chan Event {
	return s.events
}

func (s *MessageService) publish(evt Event) {
	if s.bus == nil {
		return
	}

	s.bus.Publish(evt)
}

// NOTE:necessary, because Some fields in the business layer message
// need to be converted to JSON and stored in the store layer.
func toMessage(msg store.Message) (Message, error) {
	payload, err := unmarshalMessageJSON(msg.MessageJSON)
	if err != nil {
		return Message{}, err
	}

	return Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Kind:      Kind(msg.Kind),
		Status:    StatusComplete,
		CreatedAt: msg.FinishedAt,
		UpdatedAt: msg.FinishedAt,
		Flags:     payload.Flags,
		Parts:     payload.Parts,
		System:    payload.System,
		Progress:  payload.Progress,
	}, nil
}

func buildMessageID(kind Kind, now time.Time) string {
	prefix := "message"
	switch kind {
	case KindUser:
		prefix = "user"
	case KindAssistant:
		prefix = "assistant"
	case KindSystem:
		prefix = "system"
	}
	return prefix + "-" + now.Format(time.RFC3339Nano)
}

func textContent(parts []Part) string {
	for _, part := range parts {
		if part.Type == PartTypeText {
			return part.Text
		}
	}
	return ""
}

func cloneParts(parts []Part) []Part {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]Part, len(parts))
	copy(cloned, parts)
	return cloned
}

func cloneSystemPayload(payload *SystemPayload) *SystemPayload {
	if payload == nil {
		return nil
	}
	copied := *payload
	return &copied
}

func cloneProgressPayload(payload *ProgressPayload) *ProgressPayload {
	if payload == nil {
		return nil
	}
	copied := *payload
	return &copied
}
func marshalMessageJSON(msg Message) (string, error) {
	payload := persistedMessage{
		Flags:    msg.Flags,
		Parts:    cloneParts(msg.Parts),
		System:   cloneSystemPayload(msg.System),
		Progress: cloneProgressPayload(msg.Progress),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func unmarshalMessageJSON(data string) (persistedMessage, error) {
	if data == "" {
		return persistedMessage{}, nil
	}

	var payload persistedMessage
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return persistedMessage{}, err
	}

	return payload, nil
}
