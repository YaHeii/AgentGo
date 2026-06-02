package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/segmentio/ksuid"
)

type Service struct {
	loop *QueryLoop
}

func NewService(loop *QueryLoop) *Service {
	return &Service{loop: loop}
}

func (s *Service) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	if s == nil || s.loop == nil {
		return errors.New("agent: query loop is required")
	}
	_, err := s.loop.RunQuery(ctx, sessionID, prompt)
	return err
}

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

// RunQuery orchestrates the full query loop, including history loading,
// provider turns, optional tool execution, and terminal result assembly.
func (r *QueryLoop) RunQuery(ctx context.Context, sessionID string, prompt string) (agentcontract.QueryResult, error) {
	maxTurns := lifecycle.State.MaxTurn
	if maxTurns <= 0 {
		return agentcontract.QueryResult{}, errors.New("agent: max turns must be greater than 0")
	}

	// Render the full system prompt for the LLM, but keep persisted user content raw.
	initPrompt, err := r.renderPrompt(prompt)
	if err != nil {
		return agentcontract.QueryResult{}, err
	}

	inputParts := []message.Part{
		{
			Type: message.PartTypeText,
			Text: prompt,
		},
	}
	userMessage, err := r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		SessionID:        sessionID,
		Kind:             message.KindUser,
		Provider:         lifecycle.State.Model,
		IsCompactSummary: false,
		Parts:            inputParts,
	})
	if err != nil {
		return agentcontract.QueryResult{}, err
	}

	//load history
	history, err := r.deps.App.ListHistory(ctx, sessionID)
	if err != nil {
		return agentcontract.QueryResult{}, err
	}
	historyMessages := r.preprocessHistory(history)

	//Assemble loopstate
	if !containsMessage(historyMessages, userMessage.ID) {
		historyMessages = append(historyMessages, userMessage)
	}

	loopstate := LoopState{
		Messages:  historyMessages,
		TurnCount: 1,
	}
	loopstate.Transition = "user_message_created"

	// setup AssitantMessage Placeholder
	assistantMessage, err := r.newAssistantMessage(sessionID, loopstate.TurnCount)
	loopstate.Messages = append(loopstate.Messages, assistantMessage)

	r.dispatch(agentcontract.LoopStarted, loopstate, nil)

	firstReq, err := r.buildInitialRequest(loopstate, initPrompt)
	if err != nil {
		return agentcontract.QueryResult{}, err
	}

	loopstate, err = r.runTurn(ctx, loopstate, firstReq)
	if err != nil {
		r.dispatch(agentcontract.LoopFinishFailed, loopstate, err)
		return agentcontract.QueryResult{}, err
	}

	for {
		// Tool calls are executed between provider turns and their outputs are
		// persisted as system messages before the next request is rendered.
		if loopstate.LoopStatus == agentcontract.LoopAwaitingTool {
			loopstate, err = r.executePendingTools(ctx, loopstate, sessionID)
			if err != nil {
				r.dispatch(agentcontract.LoopFinishFailed, loopstate, err)
				return agentcontract.QueryResult{}, err
			}
			continue
		}

		// A length stop means the provider has more to say, so the loop advances
		// to the next assistant turn until the configured turn budget is exhausted.
		if loopstate.LoopStatus == agentcontract.LoopCompleted && loopstate.ProviderStopReason == providercontract.StopReasonLength && loopstate.TurnCount < maxTurns {
			loopstate.TurnCount++
		} else if loopstate.LoopStatus == agentcontract.LoopCompleted {
			// 退出循环，结束本次任务
			break
		}

		// Exiting here preserves the latest assistant state instead of failing
		// when the loop hit the turn budget after a valid provider response.
		if loopstate.TurnCount > maxTurns {
			// TODO: Context Compression here
			continue
		}
		// For multi-round processing, a new request would be generate here,
		// to avoid rerender the initial md template by buildInitialRequest func
		history, err := r.deps.App.ListHistory(ctx, sessionID)
		if err != nil {
			return agentcontract.QueryResult{}, err
		}
		loopstate.Messages = r.preprocessHistory(history)
		loopstate.Transition = "history_loaded"

		assistantMessage, err := r.newAssistantMessage(sessionID, loopstate.TurnCount)
		loopstate.Messages = append(loopstate.Messages, assistantMessage)
		loopstate.Transition = "assistant_turn_initialized"

		// Later turns reuse persisted history and tool results instead of rebuilding
		// the initial system prompt on every iteration.
		req := r.buildRequest(loopstate)

		loopstate, err = r.runTurn(ctx, loopstate, req)
		if err != nil {
			r.dispatch(agentcontract.LoopFinishFailed, loopstate, err)
			return agentcontract.QueryResult{}, err
		}
	}
	// find the Placeholder Data
	assistantIndex := -1
	for i := len(loopstate.Messages) - 1; i >= 0; i-- {
		if loopstate.Messages[i].Kind == message.KindAssistant {
			assistantIndex = i
			break
		}
	}

	if assistantIndex == -1 {
		return agentcontract.QueryResult{}, errors.New("agent: assistant message not found")
	}

	r.dispatch(agentcontract.LoopCompleted, loopstate, nil)
	// TODO: ADD hooks here
	return agentcontract.QueryResult{
		SessionID:               sessionID,
		UserMessageID:           userMessage.ID,
		FinalAssistantMessageID: assistantIndex,
		Turns:                   loopstate.TurnCount,
		FinishReason:            agentcontract.LoopCompleted,
	}, nil
}

func containsMessage(messages []message.Message, id string) bool {
	for i := range messages {
		if messages[i].ID == id {
			return true
		}
	}
	return false
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

// runTurn executes one provider turn against the latest assistant placeholder,
// persists the assistant output, and translates provider outcomes into loop state.
func (r *QueryLoop) runTurn(ctx context.Context, state LoopState, req providercontract.Request) (LoopState, error) {
	//TODO: NOT to copy
	state = copyLoopState(state)

	// find the Placeholder Data
	assistantIndex := -1
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Kind == message.KindAssistant {
			assistantIndex = i
			break
		}
	}

	if assistantIndex == -1 {
		return state, errors.New("agent: assistant message not found")
	}

	turnResult, err := r.deps.Provider.RunTurn(ctx, req)
	state.ProviderStopReason = turnResult.StopReason

	assistantMessage := state.Messages[assistantIndex]
	assistantMessage.Parts = buildAssistantParts(turnResult)
	assistantMessage.UpdatedAt = r.deps.Now().UTC()
	state.Messages[assistantIndex] = assistantMessage

	//Error Handling for runturn
	if err != nil {
		// Persist partial assistant output before surfacing the provider failure so
		// the transcript still reflects what was produced before the error.
		persistedAssistant, persistErr := r.persistAssistant(ctx, assistantMessage)
		if persistErr != nil {
			return state, persistErr
		}
		state.Messages[assistantIndex] = persistedAssistant
		state.Transition = "provider_stream_failed"
		state.LoopStatus = agentcontract.LoopFinishFailed
		if errors.Is(err, context.Canceled) {
			state.LoopStatus = agentcontract.FinishReasonCancelled
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
	state.PendingToolCalls = append([]providercontract.ToolCall(nil), turnResult.ToolCalls...)

	if len(turnResult.ToolCalls) > 0 {
		// Tool calls pause the provider loop until the tool runner records results.
		state.Transition = "awaiting_tool_execution"
		state.LoopStatus = agentcontract.LoopAwaitingTool
		r.dispatch(agentcontract.LoopAwaitingTool, state, nil)
		return state, nil
	}

	state.Transition = "assistant_completed"
	state.LoopStatus = agentcontract.LoopCompleted
	r.dispatch(agentcontract.LoopCompleted, state, nil)
	return state, nil
}

// executePendingTools resolves the queued tool calls and appends each result to
// the persisted transcript as a system message for the next provider turn.
func (r *QueryLoop) executePendingTools(ctx context.Context, state LoopState, sessionID string) (LoopState, error) {
	state = copyLoopState(state)
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
	state.LoopStatus = ""
	state.Transition = "tool_results_recorded"
	return state, nil
}

// newAssistantMessage creates the placeholder message that will be filled with
// provider output and then persisted as the assistant turn record.
func (r *QueryLoop) newAssistantMessage(sessionID string, turn int) (message.Message, error) {
	now := r.deps.Now().UTC()
	var id ksuid.KSUID
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		id, err = ksuid.NewRandomWithTime(now)
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond * time.Duration(10*(attempt+1)))
	}

	if err != nil {
		return message.Message{}, fmt.Errorf("generate assistant message ID: %w", err)
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
	}, nil
}

// persistAssistant writes the latest assistant snapshot back to the message store.
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

// persistToolResult records a tool execution outcome as a system message so it
// can be replayed into later provider turns.
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

// createSystemErrorMessage stores provider failures in the transcript so the
// UI and later debugging flows can inspect the terminal error context.
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

// dispatch mirrors query-loop state changes onto the shared app event bus.
func (r *QueryLoop) dispatch(status agentcontract.LoopStatus, state LoopState, err error) {
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
