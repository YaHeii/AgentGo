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
	message "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/segmentio/ksuid"
)

type QueryLoop struct {
	messageWindow int
	deps          QueryDeps
}

func NewQueryLoop(appSvc appStore, providerSvc providerStore, d app.Dispatcher) *QueryLoop {
	return &QueryLoop{
		messageWindow: 20,
		deps: QueryDeps{
			App:        appSvc,
			Provider:   providerSvc,
			Now:        time.Now,
			dispatcher: d,
		},
	}
}

func (r *QueryLoop) RunQuery(ctx context.Context, sessionID string, prompt string) (QueryResult, error) {
	maxTurns := r.maxTurns()
	if maxTurns <= 0 {
		return QueryResult{}, errors.New("agent: max turns must be greater than 0")
	}

	inputParts := []message.Part{
		{
			Type: message.PartTypeText,
			Text: prompt,
		},
	}
	userMessage, err := r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		SessionID: sessionID,
		Kind:      message.KindUser,
		Parts:     inputParts,
	})
	if err != nil {
		return QueryResult{}, err
	}
	loopstate := newLoopState(sessionID, inputParts)
	loopstate.Messages = []message.Message{userMessage}
	loopstate.Transition = "user_message_created"
	r.dispatch(QueryStatusStarted, loopstate, nil)

	history, err := r.deps.App.ListHistory(ctx, sessionID)
	if err != nil {
		return QueryResult{}, err
	}
	loopstate.Messages = r.preprocessHistory(history)
	loopstate.Transition = "history_loaded"

	assistantMessage := r.newAssistantMessage(sessionID, loopstate.TurnCount)
	loopstate.Messages = append(loopstate.Messages, assistantMessage)
	loopstate.Transition = "assistant_turn_initialized"
	r.dispatch(QueryStatusDelta, loopstate, nil)

	initPrompt, err := r.renderPrompt(loopstate, prompt)
	if err != nil {
		return QueryResult{}, err
	}
	firstReq, err := r.buildInitialRequest(loopstate, initPrompt)
	if err != nil {
		return QueryResult{}, err
	}

	loopstate, err = r.runTurn(ctx, loopstate, firstReq)
	if err != nil {
		r.dispatch(QueryStatusFailed, loopstate, err)
		return QueryResult{}, err
	}

	for {
		if loopstate.FinishReason == FinishReasonAwaitingToolExecution {
			loopstate, err = r.executePendingTools(ctx, loopstate, sessionID)
			if err != nil {
				r.dispatch(QueryStatusFailed, loopstate, err)
				return QueryResult{}, err
			}
			continue
		}

		if loopstate.FinishReason == FinishReasonCompleted && loopstate.StopReason == providercontract.StopReasonLength && loopstate.TurnCount < maxTurns {
			loopstate.TurnCount++
		} else if loopstate.FinishReason == FinishReasonCompleted {
			r.dispatch(QueryStatusCompleted, loopstate, nil)
			return QueryResult{
				SessionID:               sessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: loopstate.AssistantMessageID,
				Turns:                   loopstate.TurnCount,
				FinishReason:            loopstate.FinishReason,
				PendingToolCalls:        append([]providercontract.ToolCall(nil), loopstate.PendingToolCalls...),
			}, nil
		}

		if loopstate.TurnCount > maxTurns {
			break
		}

		history, err := r.deps.App.ListHistory(ctx, sessionID)
		if err != nil {
			return QueryResult{}, err
		}
		loopstate.Messages = r.preprocessHistory(history)
		loopstate.Transition = "history_loaded"

		assistantMessage := r.newAssistantMessage(sessionID, loopstate.TurnCount)
		loopstate.Messages = append(loopstate.Messages, assistantMessage)
		loopstate.Transition = "assistant_turn_initialized"

		r.dispatch(QueryStatusDelta, loopstate, nil)

		req, err := r.renderLoopstate(loopstate)
		if err != nil {
			return QueryResult{}, err
		}

		loopstate, err = r.runTurn(ctx, loopstate, req)
		if err != nil {
			r.dispatch(QueryStatusFailed, loopstate, err)
			return QueryResult{}, err
		}
	}

	lastAssistantID := ""
	for i := len(loopstate.Messages) - 1; i >= 0; i-- {
		if loopstate.Messages[i].Kind == message.KindAssistant {
			lastAssistantID = loopstate.Messages[i].ID
			break
		}
	}
	r.dispatch(QueryStatusCompleted, loopstate, nil)
	return QueryResult{
		SessionID:               sessionID,
		UserMessageID:           userMessage.ID,
		FinalAssistantMessageID: lastAssistantID,
		Turns:                   loopstate.TurnCount,
		FinishReason:            FinishReasonCompleted,
	}, nil
}

func (r *QueryLoop) preprocessHistory(history []message.Message) []message.Message {
	processed := cloneMessages(history)
	if r.messageWindow > 0 && len(processed) > r.messageWindow {
		processed = processed[len(processed)-r.messageWindow:]
	}
	if lifecycle.State == nil || lifecycle.CurrentSupervisor == nil {
		return processed
	}
	if lifecycle.State.ModelLimit <= 0 || strings.TrimSpace(lifecycle.State.Model) == "" {
		return processed
	}

	for len(processed) > 1 {
		exceeded, err := r.exceedsModelLimit(lifecycle.State.Model, lifecycle.State.ModelLimit, processed)
		if err == nil && !exceeded {
			return processed
		}
		processed = processed[1:]
	}
	return processed
}

func cloneMessages(history []message.Message) []message.Message {
	if len(history) == 0 {
		return nil
	}

	copied := make([]message.Message, len(history))
	for i := range history {
		copied[i] = history[i]
		copied[i].Parts = cloneMessageParts(history[i].Parts)
	}
	return copied
}

func (r *QueryLoop) estimateMessagesTokens(model string, messages []message.Message) (int, error) {
	if lifecycle.CurrentSupervisor == nil {
		return 0, errors.New("agent: supervisor is required for token estimation")
	}
	return lifecycle.CurrentSupervisor.EstimateTokens(model, messages)
}

func (r *QueryLoop) exceedsModelLimit(model string, limit int, messages []message.Message) (bool, error) {
	builder := flattenMessages(messages)
	if len(builder) > limit {
		return true, nil
	}

	tokens, err := r.estimateMessagesTokens(model, messages)
	if err != nil {
		return false, nil
	}
	return tokens > limit, nil
}

func flattenMessages(messages []message.Message) []byte {
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
			}
		}
	}
	return builder
}

func (r *QueryLoop) runTurn(ctx context.Context, state LoopState, req providercontract.Request) (LoopState, error) {
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
	state.PendingToolCalls = append([]providercontract.ToolCall(nil), turnResult.ToolCalls...)

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

	batch := toolcontract.BatchRequest{
		Calls: make([]toolcontract.ToolCallRequest, 0, len(state.PendingToolCalls)),
	}
	permissionLevel := toolcontract.SecurityLevel(0)
	workspaceRoot := ""
	workingDir := ""
	if lifecycle.State != nil {
		permissionLevel = toolcontract.SecurityLevel(lifecycle.State.PermissionLevel)
		workspaceRoot = lifecycle.State.ProjectRoot
		workingDir = lifecycle.State.Cwd
	}
	for _, call := range state.PendingToolCalls {
		batch.Calls = append(batch.Calls, toolcontract.ToolCallRequest{
			ToolCallID:      call.ID,
			Name:            call.Name,
			Arguments:       json.RawMessage(call.Arguments),
			PermissionLevel: permissionLevel,
			Context: toolcontract.ToolCallContext{
				SessionID:     sessionID,
				TurnID:        fmt.Sprintf("turn-%d", state.TurnCount),
				WorkspaceRoot: workspaceRoot,
				WorkingDir:    workingDir,
			},
		})
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
	state.FinishReason = ""
	state.Transition = "tool_results_recorded"
	r.dispatch(QueryStatusDelta, state, nil)
	return state, nil
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

func (r *QueryLoop) persistToolResult(ctx context.Context, sessionID string, result toolcontract.ToolResult) (message.Message, error) {
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
				IsError:    result.Err != nil || result.Status != toolcontract.StatusSuccess,
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

func buildAssistantParts(turn providercontract.TurnResult) []message.Part {
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

func toProviderMessage(msg message.Message) (message.Message, bool) {
	if msg.Kind != message.KindSystem {
		return msg, true
	}
	if firstToolResultPart(msg.Parts) != nil {
		return msg, true
	}
	return message.Message{}, false
}

func firstToolResultPart(parts []message.Part) *message.ToolResultPart {
	for _, part := range parts {
		if part.Type == message.PartTypeToolResult && part.ToolResult != nil {
			return part.ToolResult
		}
	}
	return nil
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

func (r *QueryLoop) maxTurns() int {
	if lifecycle.State != nil && lifecycle.State.MaxTurn > 0 {
		return lifecycle.State.MaxTurn
	}
	return 10
}
