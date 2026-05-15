package message

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
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
func (s *MessageService) CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error) {
	now := time.Now().UTC()
	if len(params.Parts) == 0 {
		params.Parts = []messagecontract.Part{{Type: messagecontract.PartTypeText}}
	}

	msg := messagecontract.Message{
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
		return messagecontract.Message{}, err
	}

	row, err := s.messageStore.CreateMessage(ctx, CreateMessageRecordParams{
		ID:               msg.ID,
		SessionID:        msg.SessionID,
		Kind:             string(params.Kind),
		Provider:         params.Provider,
		FinishedAt:       now,
		IsCompactSummary: params.IsCompactSummary,
		MessageJSON:      messageJSON,
	})
	if err != nil {
		return messagecontract.Message{}, err
	}

	msg, err = toMessage(row)
	if err != nil {
		return messagecontract.Message{}, err
	}

	return msg, nil
}

func (s *MessageService) ListMessages(ctx context.Context, sessionID string) ([]messagecontract.Message, error) {
	rows, err := s.messageStore.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]messagecontract.Message, 0, len(rows))
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
func toMessage(msg MessageRecord) (messagecontract.Message, error) {
	payload, err := unmarshalMessageJSON(msg.MessageJSON)
	if err != nil {
		return messagecontract.Message{}, err
	}

	return messagecontract.Message{
		ID:               msg.ID,
		SessionID:        msg.SessionID,
		Kind:             messagecontract.Kind(msg.Kind),
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

func textContent(parts []messagecontract.Part) string {
	for _, part := range parts {
		if part.Type == messagecontract.PartTypeText {
			return part.Text
		}
	}
	return ""
}

func cloneParts(parts []messagecontract.Part) []messagecontract.Part {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]messagecontract.Part, len(parts))
	copy(cloned, parts)
	return cloned
}

func cloneSystemPayload(payload *messagecontract.SystemPayload) *messagecontract.SystemPayload {
	if payload == nil {
		return nil
	}
	copied := *payload
	return &copied
}

func cloneProgressPayload(payload *messagecontract.ProgressPayload) *messagecontract.ProgressPayload {
	if payload == nil {
		return nil
	}
	copied := *payload
	return &copied
}

type persistedMessage struct {
	Parts    []messagecontract.Part           `json:"parts"`
	System   *messagecontract.SystemPayload   `json:"system,omitempty"`
	Progress *messagecontract.ProgressPayload `json:"progress,omitempty"`
}

func marshalMessageJSON(msg messagecontract.Message) (string, error) {
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

func (s *MessageService) ListUserMessages(_ context.Context, _ string) ([]messagecontract.Message, error) {
	return nil, errTODO
}

func (s *MessageService) ListAllUserMessages(_ context.Context) ([]messagecontract.Message, error) {
	return nil, errTODO
}

func (s *MessageService) Delete(_ context.Context, _ string) error {
	return errTODO
}

func (s *MessageService) DeleteSessionMessages(_ context.Context, _ string) error {
	return errTODO
}

func (s *MessageService) Get(_ context.Context, _ string) (messagecontract.Message, error) {
	return messagecontract.Message{}, errTODO
}
