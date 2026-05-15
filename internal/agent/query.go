package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/tool"
	"github.com/segmentio/ksuid"
)

type QueryLoop struct {
	config QueryConfig
	deps   QueryDeps
}

func NewQueryLoop(appSvc appStore, providerSvc providerStore, d app.Dispatcher) *QueryLoop {
	return &QueryLoop{
		// TODO:setup:
		config: QueryConfig{
			MaxTurns: 10,
		},
		deps: QueryDeps{
			App:        appSvc,
			Provider:   providerSvc,
			Now:        time.Now,
			dispatcher: d,
		},
	}
}

func (r *QueryLoop) RunPrompt(ctx context.Context, sessionID string, prompt string) error {
	_, err := r.RunQuery(ctx, QueryParams{
		SessionID: sessionID,
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: prompt,
			},
		},
	})
	return err
}

func (r *QueryLoop) RunQuery(ctx context.Context, params QueryParams) (QueryResult, error) {
	if r.config.MaxTurns <= 0 {
		return QueryResult{}, errors.New("agent: max turns must be greater than 0")
	}

	inputParts := cloneMessageParts(params.InputParts)
	userMessage, err := r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		SessionID: params.SessionID,
		Kind:      message.KindUser,
		Parts:     inputParts,
	})
	if err != nil {
		return QueryResult{}, err
	}

	state := newLoopState(params)
	state.Messages = []message.Message{userMessage}
	state.Transition = "user_message_created"
	r.dispatch(QueryStatusStarted, state, nil)

	for {
		if state.TurnCount > r.config.MaxTurns {
			break
		}

		state = copyLoopState(state)
		state.Transition = "turn_started"

		history, err := r.deps.App.ListHistory(ctx, params.SessionID)
		if err != nil {
			return QueryResult{}, err
		}
		state.Messages = history
		state.Transition = "history_loaded"

		assistantMessage := r.newAssistantMessage(params.SessionID, state.TurnCount)
		state.Messages = append(state.Messages, assistantMessage)
		state.Transition = "assistant_turn_initialized"
		r.dispatch(QueryStatusDelta, state, nil)

		tools := r.listTools(ctx)
		req, err := r.buildProviderRequest(state.Messages, tools)
		if err != nil {
			return QueryResult{}, err
		}

		state, err = r.runTurn(ctx, state, req)
		if err != nil {
			r.dispatch(QueryStatusFailed, state, err)
			return QueryResult{}, err
		}

		if state.FinishReason == FinishReasonAwaitingToolExecution {
			state, err = r.executePendingTools(ctx, state, params.SessionID)
			if err != nil {
				r.dispatch(QueryStatusFailed, state, err)
				return QueryResult{}, err
			}
			continue
		}

		if state.FinishReason == FinishReasonCompleted && state.StopReason == provider.StopReasonLength && state.TurnCount < r.config.MaxTurns {
			state = copyLoopState(state)
			state.TurnCount++
			continue
		}

		if state.FinishReason == FinishReasonCompleted {
			r.dispatch(QueryStatusCompleted, state, nil)
			return QueryResult{
				SessionID:               params.SessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: state.AssistantMessageID,
				Turns:                   state.TurnCount,
				FinishReason:            state.FinishReason,
				PendingToolCalls:        append([]provider.ToolCall(nil), state.PendingToolCalls...),
			}, nil
		}
	}

	lastAssistantID := ""
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Kind == message.KindAssistant {
			lastAssistantID = state.Messages[i].ID
			break
		}
	}
	r.dispatch(QueryStatusCompleted, state, nil)
	return QueryResult{
		SessionID:               params.SessionID,
		UserMessageID:           userMessage.ID,
		FinalAssistantMessageID: lastAssistantID,
		Turns:                   state.TurnCount,
		FinishReason:            FinishReasonCompleted,
	}, nil
}

func (r *QueryLoop) runTurn(ctx context.Context, state LoopState, req provider.Request) (LoopState, error) {
	state = copyLoopState(state)

	assistantIndex := latestAssistantIndex(state.Messages)
	if assistantIndex < 0 {
		return state, errors.New("agent: assistant message not found")
	}

	turnResult, err := r.deps.Provider.RunTurn(ctx, req)
	state.StopReason = turnResult.StopReason

	assistantMessage := state.Messages[assistantIndex]
	assistantMessage.Parts = buildAssistantParts(turnResult)
	assistantMessage.UpdatedAt = r.deps.Now().UTC()
	state.Messages[assistantIndex] = assistantMessage

	if err != nil {
		persistedAssistant, persistErr := r.persistAssistant(ctx, assistantMessage)
		if persistErr != nil {
			return state, persistErr
		}
		state.Messages[assistantIndex] = persistedAssistant
		state.AssistantMessageID = persistedAssistant.ID
		state.Transition = "stream_failed"
		state.FinishReason = FinishReasonFailed
		if errors.Is(err, context.Canceled) {
			state.FinishReason = FinishReasonCancelled
		}

		systemMessage, persistErr := r.createSystemErrorMessage(ctx, assistantMessage.SessionID, err)
		if persistErr != nil {
			return state, persistErr
		}
		state.Messages = append(state.Messages, systemMessage)
		state.Transition = "provider_error_recorded"
		return state, err
	}

	persistedAssistant, err := r.persistAssistant(ctx, assistantMessage)
	if err != nil {
		return state, err
	}
	state.Messages[assistantIndex] = persistedAssistant
	state.AssistantMessageID = persistedAssistant.ID
	state.PendingToolCalls = append([]provider.ToolCall(nil), turnResult.ToolCalls...)

	if len(turnResult.ToolCalls) > 0 {
		state.Transition = "awaiting_tool_execution"
		state.FinishReason = FinishReasonAwaitingToolExecution
		r.dispatch(QueryStatusDelta, state, nil)
		return state, nil
	}

	state.Transition = "assistant_completed"
	state.FinishReason = FinishReasonCompleted
	r.dispatch(QueryStatusDelta, state, nil)
	return state, nil
}

func (r *QueryLoop) executePendingTools(ctx context.Context, state LoopState, sessionID string) (LoopState, error) {
	state = copyLoopState(state)
	if len(state.PendingToolCalls) == 0 {
		state.FinishReason = FinishReasonCompleted
		return state, nil
	}
	if r.deps.App == nil {
		return state, errors.New("agent: tool runner is required")
	}

	batch := tool.BatchRequest{
		Calls: make([]tool.ToolCallRequest, 0, len(state.PendingToolCalls)),
	}
	for _, call := range state.PendingToolCalls {
		batch.Calls = append(batch.Calls, tool.NewToolCallRequest(
			call.ID,
			call.Name,
			json.RawMessage(call.Arguments),
			tool.ToolCallContext{
				SessionID:     sessionID,
				TurnID:        fmt.Sprintf("turn-%d", state.TurnCount),
				WorkspaceRoot: lifecycle.GetState().ProjectRoot,
				WorkingDir:    lifecycle.GetState().Cwd,
			},
		))
	}

	results, err := r.deps.App.CallTools(ctx, batch)
	if err != nil {
		return state, err
	}

	for _, result := range results {
		msg, err := r.persistToolResult(ctx, sessionID, result)
		if err != nil {
			return state, err
		}
		state.Messages = append(state.Messages, msg)
	}
	state.PendingToolCalls = nil
	state.Transition = "tool_results_recorded"
	r.dispatch(QueryStatusDelta, state, nil)
	return state, nil
}

func (r *QueryLoop) buildProviderRequest(history []message.Message, tools []tool.Metadata) (provider.Request, error) {
	requestMessages := trimPendingAssistant(history)
	state := lifecycle.GetState()
	promptCtx := PromptContext{
		AppVersion:  state.AppVersion,
		ProjectRoot: state.ProjectRoot,
		Cwd:         state.Cwd,
		Tools:       tools,
		History:     buildPromptHistory(requestMessages),
		UserInput:   latestUserInput(requestMessages),
	}

	systemPrompt, err := r.renderPrompt(promptCtx)
	if err != nil {
		return provider.Request{}, err
	}

	req := provider.Request{
		Messages: []provider.Message{
			{
				Role:    provider.RoleSystem,
				Content: systemPrompt,
			},
		},
	}
	for _, msg := range requestMessages {
		providerMsg, ok := toProviderMessage(msg)
		if !ok {
			continue
		}
		req.Messages = append(req.Messages, providerMsg)
	}
	req.Tools = make([]provider.ToolDefinition, 0, len(tools))
	for _, meta := range tools {
		req.Tools = append(req.Tools, provider.ToolDefinition{
			Name:        meta.Name,
			Description: meta.Description,
			Parameters:  meta.Parameters,
		})
	}
	return req, nil
}

func (r *QueryLoop) listTools(ctx context.Context) []tool.Metadata {
	if r.deps.App == nil {
		return nil
	}
	return r.deps.App.ListTools(ctx)
}

func (r *QueryLoop) newAssistantMessage(sessionID string, turn int) message.Message {
	now := r.deps.Now().UTC()
	id, err := ksuid.NewRandomWithTime(now)
	if err != nil {
		return message.Message{
			ID:        fmt.Sprintf("assistant-turn-%d-%d", turn, now.UnixNano()),
			SessionID: sessionID,
			Kind:      message.KindAssistant,
			CreatedAt: now,
			UpdatedAt: now,
			Parts: []message.Part{
				{
					Type: message.PartTypeText,
					Text: "",
				},
			},
		}
	}
	return message.Message{
		ID:        id.String(),
		SessionID: sessionID,
		Kind:      message.KindAssistant,
		CreatedAt: now,
		UpdatedAt: now,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "",
			},
		},
	}
}

func (r *QueryLoop) persistAssistant(ctx context.Context, assistant message.Message) (message.Message, error) {
	return r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		ID:               assistant.ID,
		SessionID:        assistant.SessionID,
		Kind:             assistant.Kind,
		IsCompactSummary: assistant.IsCompactSummary,
		Parts:            cloneMessageParts(assistant.Parts),
		System:           assistant.System,
		Progress:         assistant.Progress,
	})
}

func (r *QueryLoop) persistToolResult(ctx context.Context, sessionID string, result tool.ToolResult) (message.Message, error) {
	content := strings.TrimSpace(result.Content)
	parts := []message.Part{
		{
			Type: message.PartTypeText,
			Text: content,
		},
		{
			Type: message.PartTypeToolResult,
			ToolResult: &message.ToolResultPart{
				ToolCallID: result.ToolCallID,
				Content:    content,
				IsError:    result.Err != nil || result.Status != tool.StatusSuccess,
			},
		},
	}
	return r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		SessionID: sessionID,
		Kind:      message.KindSystem,
		Parts:     parts,
	})
}

func (r *QueryLoop) createSystemErrorMessage(ctx context.Context, sessionID string, providerErr error) (message.Message, error) {
	text := "provider error"
	if providerErr != nil {
		text = "provider error: " + providerErr.Error()
	}
	return r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		SessionID: sessionID,
		Kind:      message.KindSystem,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: text,
			},
		},
		System: &message.SystemPayload{
			Subtype: "provider_error",
			Level:   "error",
		},
	})
}

func (r *QueryLoop) dispatch(status QueryStatus, state LoopState, err error) {
	if r.deps.dispatcher == nil {
		return
	}
	r.deps.dispatcher.Dispatch(app.BaseEvent{
		T: app.EventAgent,
		Payload: QueryEvent{
			Status: status,
			State:  copyLoopState(state),
			Err:    err,
		},
	})
}

func latestAssistantIndex(messages []message.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Kind == message.KindAssistant {
			return i
		}
	}
	return -1
}

func buildAssistantParts(turn provider.TurnResult) []message.Part {
	parts := make([]message.Part, 0, 2+len(turn.ToolCalls))
	if turn.Text != "" || turn.Refusal != "" || len(turn.ToolCalls) == 0 {
		parts = append(parts, message.Part{
			Type: message.PartTypeText,
			Text: turn.Text + turn.Refusal,
		})
	}
	if turn.Reasoning != "" {
		parts = append(parts, message.Part{
			Type: message.PartTypeThinking,
			Thinking: &message.ThinkingPart{
				Content: turn.Reasoning,
			},
		})
	}
	for _, call := range turn.ToolCalls {
		parts = append(parts, message.Part{
			Type: message.PartTypeToolCall,
			ToolCall: &message.ToolCallPart{
				ID:     call.ID,
				Name:   call.Name,
				Input:  call.Arguments,
				Status: "completed",
			},
		})
	}
	return parts
}

func buildPromptHistory(history []message.Message) []PromptMessage {
	out := make([]PromptMessage, 0, len(history))
	for _, msg := range history {
		role := string(msg.Kind)
		content := flattenMessageParts(msg.Parts)
		if content == "" {
			continue
		}
		out = append(out, PromptMessage{
			Role:    role,
			Content: content,
		})
	}
	return out
}

func latestUserInput(history []message.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Kind == message.KindUser {
			return flattenMessageParts(history[i].Parts)
		}
	}
	return ""
}

func flattenMessageParts(parts []message.Part) string {
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case message.PartTypeText:
			if strings.TrimSpace(part.Text) != "" {
				segments = append(segments, part.Text)
			}
		case message.PartTypeThinking:
			if part.Thinking != nil && strings.TrimSpace(part.Thinking.Content) != "" {
				segments = append(segments, part.Thinking.Content)
			}
		case message.PartTypeToolResult:
			if part.ToolResult != nil && strings.TrimSpace(part.ToolResult.Content) != "" {
				segments = append(segments, part.ToolResult.Content)
			}
		}
	}
	return strings.Join(segments, "\n")
}

func toProviderMessage(msg message.Message) (provider.Message, bool) {
	switch msg.Kind {
	case message.KindUser:
		return provider.Message{
			Role:    provider.RoleUser,
			Content: flattenMessageParts(msg.Parts),
		}, true
	case message.KindAssistant:
		out := provider.Message{
			Role:    provider.RoleAssistant,
			Content: flattenMessageParts(msg.Parts),
		}
		for _, part := range msg.Parts {
			if part.Type == message.PartTypeToolCall && part.ToolCall != nil {
				out.ToolCalls = append(out.ToolCalls, provider.ToolCall{
					ID:        part.ToolCall.ID,
					Name:      part.ToolCall.Name,
					Arguments: part.ToolCall.Input,
				})
			}
		}
		return out, true
	case message.KindSystem:
		toolResult := firstToolResultPart(msg.Parts)
		if toolResult != nil {
			return provider.Message{
				Role:       provider.RoleTool,
				ToolCallID: toolResult.ToolCallID,
				Content:    flattenMessageParts(msg.Parts),
			}, true
		}
		return provider.Message{}, false
	default:
		return provider.Message{}, false
	}
}

func firstToolResultPart(parts []message.Part) *message.ToolResultPart {
	for _, part := range parts {
		if part.Type == message.PartTypeToolResult && part.ToolResult != nil {
			return part.ToolResult
		}
	}
	return nil
}

func trimPendingAssistant(history []message.Message) []message.Message {
	if len(history) == 0 {
		return nil
	}

	trimmed := history
	last := history[len(history)-1]
	if last.Kind == message.KindAssistant && isPendingAssistantMessage(last) {
		trimmed = history[:len(history)-1]
	}

	out := make([]message.Message, len(trimmed))
	copy(out, trimmed)
	return out
}

func isPendingAssistantMessage(msg message.Message) bool {
	if len(msg.Parts) == 0 {
		return true
	}
	if len(msg.Parts) > 1 {
		return false
	}

	part := msg.Parts[0]
	return part.Type == message.PartTypeText && strings.TrimSpace(part.Text) == ""
}

func cloneMessageParts(parts []message.Part) []message.Part {
	if len(parts) == 0 {
		return nil
	}

	copied := make([]message.Part, len(parts))
	for i := range parts {
		copied[i] = parts[i]
		if parts[i].Image != nil {
			image := *parts[i].Image
			copied[i].Image = &image
		}
		if parts[i].ToolCall != nil {
			toolCall := *parts[i].ToolCall
			copied[i].ToolCall = &toolCall
		}
		if parts[i].ToolResult != nil {
			toolResult := *parts[i].ToolResult
			copied[i].ToolResult = &toolResult
		}
		if parts[i].Thinking != nil {
			thinking := *parts[i].Thinking
			copied[i].Thinking = &thinking
		}
	}
	return copied
}
