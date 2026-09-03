package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/pkoukk/tiktoken-go"
)

const contextSafetyMargin = 256
const compactionThresholdPercent = 90
const compactedHistoryPercent = 70
const compactSummaryPercent = 10

type contextBudget struct {
	ModelLimit          int
	OutputTokens        int
	SafetyMargin        int
	FixedTokens         int
	HistoryBudget       int
	EstimatedTokens     int
	CompactionThreshold int
}

func (b contextBudget) ShouldCompact(estimatedTokens int) bool {
	return estimatedTokens >= b.CompactionThreshold
}

var countTokens = countTokensWithTiktoken

func calculateContextBudget(model string, modelLimit, outputTokens int, systemPrompt string, tools []toolcontract.Metadata) (contextBudget, error) {
	if modelLimit <= 0 {
		return contextBudget{}, errors.New("agent: model context window must be greater than 0")
	}
	if outputTokens < 0 {
		return contextBudget{}, errors.New("agent: output token budget cannot be negative")
	}

	systemTokens, err := countTokens(model, systemPrompt)
	if err != nil {
		return contextBudget{}, err
	}
	toolData, err := json.Marshal(tools)
	if err != nil {
		return contextBudget{}, fmt.Errorf("marshal tool definitions: %w", err)
	}
	toolTokens, err := countTokens(model, string(toolData))
	if err != nil {
		return contextBudget{}, err
	}
	fixedTokens := systemTokens + toolTokens + messageOverhead

	historyBudget := modelLimit - outputTokens - contextSafetyMargin - fixedTokens
	if historyBudget < 0 {
		historyBudget = 0
	}

	return contextBudget{
		ModelLimit:          modelLimit,
		OutputTokens:        outputTokens,
		SafetyMargin:        contextSafetyMargin,
		FixedTokens:         fixedTokens,
		HistoryBudget:       historyBudget,
		CompactionThreshold: modelLimit * compactionThresholdPercent / 100,
	}, nil
}

const messageOverhead = 4

func selectHistoryByTokenBudget(model string, history []messagecontract.Message, budget int) ([]messagecontract.Message, error) {
	if budget < 0 {
		return nil, errors.New("agent: history token budget cannot be negative")
	}
	units := historyUnits(history)
	selectedUnits := make([][]messagecontract.Message, 0, len(units))
	used := 0
	for i := len(units) - 1; i >= 0; i-- {
		unit := units[i]
		unitTokens, err := countMessagesTokens(model, unit)
		if err != nil {
			return nil, err
		}
		if used+unitTokens > budget {
			if len(selectedUnits) == 0 {
				return nil, fmt.Errorf("agent: latest history unit exceeds token budget: %d > %d", unitTokens, budget)
			}
			continue
		}
		used += unitTokens
		selectedUnits = append(selectedUnits, unit)
	}

	selected := make([]messagecontract.Message, 0, len(history))
	for i := len(selectedUnits) - 1; i >= 0; i-- {
		selected = append(selected, selectedUnits[i]...)
	}
	return selected, nil
}

func historyUnits(history []messagecontract.Message) [][]messagecontract.Message {
	units := make([][]messagecontract.Message, 0, len(history))
	for i := 0; i < len(history); {
		msg := history[i]
		if msg.Kind == messagecontract.KindSystem && firstToolResultPart(msg.Parts) != nil {
			i++
			continue
		}

		unit := []messagecontract.Message{msg}
		if msg.Kind == messagecontract.KindAssistant {
			callIDs := toolCallIDs(msg.Parts)
			j := i + 1
			for j < len(history) && history[j].Kind == messagecontract.KindSystem {
				result := firstToolResultPart(history[j].Parts)
				if result == nil || !containsString(callIDs, result.ToolCallID) {
					break
				}
				unit = append(unit, history[j])
				j++
			}
			i = j
		} else {
			i++
		}
		units = append(units, unit)
	}
	return units
}

func countMessagesTokens(model string, messages []messagecontract.Message) (int, error) {
	tokens := 0
	for _, msg := range messages {
		contentTokens, err := countMessageContentTokens(model, msg.Parts)
		if err != nil {
			return 0, err
		}
		tokens += contentTokens + messageOverhead
	}
	return tokens, nil
}

func countMessageContentTokens(model string, parts []messagecontract.Part) (int, error) {
	var content strings.Builder
	for _, part := range parts {
		switch part.Type {
		case messagecontract.PartTypeText:
			content.WriteString(part.Text)
		case messagecontract.PartTypeThinking:
			if part.Thinking != nil {
				content.WriteString(part.Thinking.Content)
				content.WriteString(part.Thinking.Summary)
			}
		case messagecontract.PartTypeToolCall:
			if part.ToolCall != nil {
				content.WriteString(part.ToolCall.Name)
				content.WriteString(part.ToolCall.Input)
			}
		case messagecontract.PartTypeToolResult:
			if part.ToolResult != nil {
				content.WriteString(part.ToolResult.Content)
			}
		case messagecontract.PartTypeSummary:
			content.WriteString(part.Text)
		}
	}
	return countTokens(model, content.String())
}

func countTokensWithTiktoken(model, text string) (int, error) {
	encoding, err := tokenizerForModel(model)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	return len(encoding.Encode(text, nil, nil)), nil
}

func toolCallIDs(parts []messagecontract.Part) []string {
	ids := make([]string, 0)
	for _, part := range parts {
		if part.Type == messagecontract.PartTypeToolCall && part.ToolCall != nil && part.ToolCall.ID != "" {
			ids = append(ids, part.ToolCall.ID)
		}
	}
	return ids
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func buildCompactSummaryMessage(sessionID string, messages []messagecontract.Message) messagecontract.Message {
	var content strings.Builder
	content.WriteString("Compressed conversation summary, newest first:\n")
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		text := strings.TrimSpace(flattenMessageParts(msg.Parts))
		if text == "" {
			continue
		}
		content.WriteString(string(msg.Kind))
		content.WriteString(": ")
		content.WriteString(text)
		content.WriteByte('\n')
	}
	return messagecontract.Message{
		SessionID:        sessionID,
		Kind:             messagecontract.KindSystem,
		IsCompactSummary: true,
		Parts: []messagecontract.Part{{
			Type: messagecontract.PartTypeSummary,
			Text: content.String(),
		}},
	}
}

func truncateTextToTokenBudget(model, text string, budget int) (string, error) {
	if budget <= 0 || strings.TrimSpace(text) == "" {
		return "", nil
	}
	tokens, err := countTokens(model, text)
	if err != nil {
		return "", err
	}
	if tokens <= budget {
		return text, nil
	}

	const suffix = "\n[summary truncated]"
	suffixTokens, err := countTokens(model, suffix)
	if err != nil {
		return "", err
	}
	if suffixTokens >= budget {
		return truncateTextToTokenBudget(model, suffix, budget)
	}
	contentBudget := budget - suffixTokens
	runes := []rune(text)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		tokens, err = countTokens(model, string(runes[:mid]))
		if err != nil {
			return "", err
		}
		if tokens <= contentBudget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return strings.TrimSpace(string(runes[:low])) + suffix, nil
}

func tokenizerForModel(model string) (*tiktoken.Tiktoken, error) {
	// Kept local to the agent package so budget selection does not depend on
	// lifecycle's global supervisor.
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
		return nil, errors.New("agent: model encoding not supported")
	}
}
