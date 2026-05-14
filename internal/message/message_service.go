package message

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/store"
	"github.com/segmentio/ksuid"
)

var errTODO = errors.New("TODO: not implemented")

type MessageService struct {
	messageStore messageStore
}

func NewMessageService(st messageStore) *MessageService {
	return &MessageService{
		messageStore: st,
	}
}

// TODO:Can the sessionID be passed into the ctx file? Is the ctx reset when the agent creates a single session?
func (s *MessageService) CreateMessage(ctx context.Context, params CreateMessageParams) (Message, error) {
	now := time.Now().UTC()
	if len(params.Parts) == 0 {
		params.Parts = []Part{{Type: PartTypeText}}
	}

	msg := Message{
		ID:               params.ID,
		SessionID:        params.SessionID,
		Kind:             params.Kind,
		CreatedAt:        now,
		UpdatedAt:        now,
		IsCompactSummary: params.IsCompactSummary,
		Parts:            cloneParts(params.Parts),
		System:           cloneSystemPayload(params.System),
		Progress:         cloneProgressPayload(params.Progress),
	}
	if msg.ID == "" {
		msg.ID = buildMessageID(now)
	}

	messageJSON, err := marshalMessageJSON(msg)
	if err != nil {
		return Message{}, err
	}

	row, err := s.messageStore.CreateMessage(ctx, store.CreateMessageParams{
		ID:               msg.ID,
		SessionID:        msg.SessionID,
		Kind:             string(params.Kind),
		Provider:         params.Provider,
		FinishedAt:       now,
		IsCompactSummary: params.IsCompactSummary,
		MessageJSON:      messageJSON,
	})
	if err != nil {
		return Message{}, err
	}

	msg, err = toMessage(row)
	if err != nil {
		return Message{}, err
	}

	return msg, nil
}

func (s *MessageService) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.messageStore.ListMessages(ctx, sessionID)
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

// NOTE:necessary, because Some fields in the business layer message
// need to be converted to JSON and stored in the store layer.
func toMessage(msg store.Message) (Message, error) {
	payload, err := unmarshalMessageJSON(msg.MessageJSON)
	if err != nil {
		return Message{}, err
	}

	return Message{
		ID:               msg.ID,
		SessionID:        msg.SessionID,
		Kind:             Kind(msg.Kind),
		CreatedAt:        msg.FinishedAt,
		UpdatedAt:        msg.FinishedAt,
		IsCompactSummary: msg.IsCompactSummary,
		Parts:            payload.Parts,
		System:           payload.System,
		Progress:         payload.Progress,
	}, nil
}

func buildMessageID(now time.Time) string {
	id, err := ksuid.NewRandomWithTime(now)
	if err == nil {
		return id.String()
	}
	return "message-" + now.Format(time.RFC3339Nano)
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

func (s *MessageService) Get(_ context.Context, _ string) (Message, error) {
	return Message{}, errTODO
}
