package message

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/store"
)

var errTODO = errors.New("TODO: not implemented")

type MessageService struct {
	store  store.Store
	bus    bus.Bus[Event]
	events <-chan Event
}

func NewMessageService(st store.Store) *MessageService {
	b := bus.NewBus[Event](128)

	return &MessageService{
		store:  st,
		bus:    b,
		events: b.Subscribe(context.Background()),
	}
}

var _ Service = (*MessageService)(nil)

func (s *MessageService) Create(ctx context.Context, sessionID string, params CreateMessageParams) (Message, error) {
	now := time.Now().UTC()
	if len(params.Parts) == 0 {
		params.Parts = []Part{{Type: PartTypeText}}
	}
	if params.Status == "" {
		params.Status = StatusComplete
	}

	row, err := s.store.CreateMessage(ctx, store.CreateMessageParams{
		ID:        buildMessageID(params.Kind, now),
		SessionID: sessionID,
		Role:      toStoreRole(params.Kind),
		Content:   textContent(params.Parts),
		Status:    toStoreStatus(params.Status),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return Message{}, err
	}

	msg := toMessage(row)
	msg.ParentID = params.ParentID
	msg.Flags = params.Flags
	msg.Parts = cloneParts(params.Parts)
	msg.System = cloneSystemPayload(params.System)
	msg.Progress = cloneProgressPayload(params.Progress)
	msg.Status = params.Status

	s.publish(MessageCreatedEvent{Message: msg})

	return msg, nil
}

func (s *MessageService) Update(ctx context.Context, message Message) error {
	updatedAt := message.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.store.UpdateMessage(ctx, store.UpdateMessageParams{
		ID:        message.ID,
		Content:   textContent(message.Parts),
		Status:    toStoreStatus(message.Status),
		UpdatedAt: updatedAt,
	})
	if err != nil {
		return err
	}

	message.UpdatedAt = updatedAt
	s.publish(MessageCompletedEvent{Message: message})

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
		messages = append(messages, toMessage(row))
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

func toMessage(msg store.Message) Message {
	return Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Kind:      toKind(msg.Role),
		Origin:    toOrigin(msg.Role),
		Status:    toStatus(msg.Status),
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: msg.Content,
			},
		},
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}
}

func toKind(role string) Kind {
	switch role {
	case "user":
		return KindUser
	case "assistant":
		return KindAssistant
	default:
		return KindSystem
	}
}

func toOrigin(role string) Origin {
	switch role {
	case "user":
		return OriginHuman
	case "assistant":
		return OriginModel
	default:
		return OriginSystem
	}
}

func toStoreRole(kind Kind) string {
	switch kind {
	case KindAssistant:
		return "assistant"
	case KindUser:
		return "user"
	default:
		return "system"
	}
}

func toStoreStatus(status Status) store.MessageStatus {
	switch status {
	case StatusStreaming:
		return store.MessageStatusStreaming
	case StatusCancelled:
		return store.MessageStatusCancelled
	case StatusFailed:
		return store.MessageStatusFailed
	default:
		return store.MessageStatusComplete
	}
}

func toStatus(status store.MessageStatus) Status {
	switch status {
	case store.MessageStatusStreaming:
		return StatusStreaming
	case store.MessageStatusCancelled:
		return StatusCancelled
	case store.MessageStatusFailed:
		return StatusFailed
	default:
		return StatusComplete
	}
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
