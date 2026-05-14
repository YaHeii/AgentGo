package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/tool"
	grepTool "github.com/YaHeii/agentGo/internal/tool/grep"
	"github.com/pkoukk/tiktoken-go"
	"github.com/segmentio/ksuid"
)

const defaultAppVersion = "0.0.1"

type Supervisor struct {
	dispatcher app.Dispatcher
	toolSvc    *tool.Service
	estimator  contextEstimator
	cfg        Config
}

type contextEstimator interface {
	Estimate(model string, messages []message.Message) (int, error)
}

type tiktokenEstimator struct{}

func NewSupervisor(dispatcher app.Dispatcher, config Config) *Supervisor {
	return &Supervisor{
		dispatcher: dispatcher,
		cfg:        config,
		estimator:  tiktokenEstimator{},
	}
}

func (s *Supervisor) Initialize(ctx context.Context) error {
	if State == nil {
		State = &GlobalState{}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	projectRoot, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}

	startTime := time.Now().UTC()
	sessionID, err := ksuid.NewRandomWithTime(startTime)
	if err != nil {
		return err
	}

	s.toolSvc = tool.NewService(grepTool.NewGrepTool(projectRoot))
	State.initialize(GlobalState{
		AppVersion:  defaultAppVersion,
		StartTime:   startTime.Format(time.RFC3339Nano),
		Cwd:         cwd,
		ProjectRoot: projectRoot,
		SessionID:   sessionID.String(),
		InitialEnv:  loadEnvironmentSnapshot(),
		ModelLimit:  normalizeContextWindow(s.cfg.ContextWindow),
		MaxTurn:     normalizeContextWindow(s.cfg.MaxTurn),
		KnownTools:  toToolSnapshots(s.toolSvc.ListTools(ctx)),
	})
	return nil
}

func (s *Supervisor) Run(ctx context.Context) {
	if s.dispatcher == nil {
		<-ctx.Done()
		return
	}

	events := s.dispatcher.Subscribe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			s.handleEvent(evt)
		}
	}
}

func (s *Supervisor) handleEvent(evt app.Event) {
	switch evt.Type() {
	case app.EventProvider:
		providerEvent, ok := evt.Data().(provider.StreamEvent)
		if !ok {
			return
		}
		if providerEvent.Type == provider.StreamEventUsageAvailable && providerEvent.Usage != nil && State != nil {
			State.applyUsage(
				providerEvent.Usage.PromptTokens,
				providerEvent.Usage.CompletionTokens,
				providerEvent.Usage.TotalTokens,
			)
		}
	case app.EventAgent:
		messages, ok := extractMessagesFromAgentPayload(evt.Data())
		if !ok || State == nil {
			return
		}
		tokens, chars, messageCount := estimateContextUsage(s.cfg.Model, messages, s.estimator)
		State.setContextEstimate(tokens, chars, messageCount)
	}
}

func extractMessagesFromAgentPayload(payload any) ([]message.Message, bool) {
	value := reflect.ValueOf(payload)
	if !value.IsValid() {
		return nil, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, false
	}

	stateField := value.FieldByName("State")
	if !stateField.IsValid() {
		return nil, false
	}
	if stateField.Kind() == reflect.Pointer {
		if stateField.IsNil() {
			return nil, false
		}
		stateField = stateField.Elem()
	}
	if stateField.Kind() != reflect.Struct {
		return nil, false
	}

	messagesField := stateField.FieldByName("Messages")
	if !messagesField.IsValid() || !messagesField.CanInterface() {
		return nil, false
	}
	messages, ok := messagesField.Interface().([]message.Message)
	if !ok {
		return nil, false
	}

	copied := make([]message.Message, len(messages))
	copy(copied, messages)
	return copied, true
}

func estimateContextUsage(model string, messages []message.Message, estimator contextEstimator) (int, int, int) {
	messageCount := len(messages)
	if messageCount == 0 {
		return 0, 0, 0
	}

	var builder []byte
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch part.Type {
			case message.PartTypeText:
				builder = append(builder, part.Text...)
			case message.PartTypeThinking:
				if part.Thinking != nil {
					builder = append(builder, part.Thinking.Content...)
					builder = append(builder, part.Thinking.Summary...)
				}
			case message.PartTypeToolCall:
				if part.ToolCall != nil {
					builder = append(builder, part.ToolCall.Name...)
					builder = append(builder, part.ToolCall.Input...)
				}
			case message.PartTypeToolResult:
				if part.ToolResult != nil {
					builder = append(builder, part.ToolResult.Content...)
				}
			case message.PartTypeAttachment:
				if part.Attachment != nil {
					builder = append(builder, part.Attachment.Name...)
					builder = append(builder, part.Attachment.Path...)
				}
			case message.PartTypeSummary:
				if part.Summary != nil {
					builder = append(builder, part.Summary.Content...)
				}
			}
		}
	}

	chars := len(builder)
	if chars == 0 {
		return 0, 0, messageCount
	}

	if estimator == nil {
		return 0, chars, messageCount
	}

	tokens, err := estimator.Estimate(model, messages)
	if err != nil {
		return 0, chars, messageCount
	}
	return tokens, chars, messageCount
}

func (tiktokenEstimator) Estimate(model string, messages []message.Message) (int, error) {
	var builder []byte
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch part.Type {
			case message.PartTypeText:
				builder = append(builder, part.Text...)
			case message.PartTypeThinking:
				if part.Thinking != nil {
					builder = append(builder, part.Thinking.Content...)
					builder = append(builder, part.Thinking.Summary...)
				}
			case message.PartTypeToolCall:
				if part.ToolCall != nil {
					builder = append(builder, part.ToolCall.Name...)
					builder = append(builder, part.ToolCall.Input...)
				}
			case message.PartTypeToolResult:
				if part.ToolResult != nil {
					builder = append(builder, part.ToolResult.Content...)
				}
			case message.PartTypeAttachment:
				if part.Attachment != nil {
					builder = append(builder, part.Attachment.Name...)
					builder = append(builder, part.Attachment.Path...)
				}
			case message.PartTypeSummary:
				if part.Summary != nil {
					builder = append(builder, part.Summary.Content...)
				}
			}
		}
	}

	encoding, err := tokenizerForModel(model)
	if err != nil {
		return 0, err
	}
	return len(encoding.Encode(string(builder), nil, nil)), nil
}

func tokenizerForModel(model string) (*tiktoken.Tiktoken, error) {
	if encoding, err := tiktoken.EncodingForModel(model); err == nil {
		return encoding, nil
	}

	trimmed := strings.TrimSpace(model)
	switch {
	case trimmed == "gpt-4o-mini", strings.HasPrefix(trimmed, "gpt-4o-mini-"):
		return tiktoken.GetEncoding("o200k_base")
	case strings.HasPrefix(trimmed, "gpt-4o"):
		return tiktoken.GetEncoding("o200k_base")
	case strings.HasPrefix(trimmed, "gpt-4"), strings.HasPrefix(trimmed, "gpt-3.5"):
		return tiktoken.GetEncoding("cl100k_base")
	default:
		return nil, errors.New("model encoding not supported")
	}
}

func loadEnvironmentSnapshot() map[string]string {
	env := os.Environ()
	out := make(map[string]string, len(env))
	for _, entry := range env {
		for i := 0; i < len(entry); i++ {
			if entry[i] != '=' {
				continue
			}
			out[entry[:i]] = entry[i+1:]
			break
		}
	}
	return out
}

func toToolSnapshots(metas []tool.Metadata) []ToolSnapshot {
	if len(metas) == 0 {
		return nil
	}

	snapshots := make([]ToolSnapshot, 0, len(metas))
	for _, meta := range metas {
		snapshots = append(snapshots, ToolSnapshot{
			Name:              meta.Name,
			Description:       meta.Description,
			Enabled:           meta.Enabled,
			SecurityLevel:     string(meta.SecurityLevel),
			IsConcurrencySafe: meta.IsConcurrencySafe,
		})
	}
	return snapshots
}

func normalizeContextWindow(v int64) int {
	if v <= 0 {
		return 0
	}
	return int(v)
}
