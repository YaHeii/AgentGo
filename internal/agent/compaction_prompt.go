package agent

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"strings"
	"text/template"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
)

//go:embed template/compaction.md.tpl
var compactionPromptFS embed.FS

type compactionPromptData struct {
	CurrentTask     string
	ExistingSummary string
	RecentContext   []PromptMessage
	Candidates      []PromptMessage
	TokenBudget     int
}

type compactionRequest struct {
	CurrentTask     string
	ExistingSummary string
	RecentContext   []messagecontract.Message
	Candidates      []messagecontract.Message
	Model           string
	TokenBudget     int
}

type compactionResult struct {
	SummaryText string
	SourceIDs   []string
	TokenCount  int
}

func (r *QueryLoop) renderCompactionPrompt(currentTask, existingSummary string, recentContext, candidates []messagecontract.Message, tokenBudget int) (string, error) {
	data := compactionPromptData{
		CurrentTask:     strings.TrimSpace(currentTask),
		ExistingSummary: strings.TrimSpace(existingSummary),
		RecentContext:   formatPromptMessages(recentContext),
		Candidates:      formatPromptMessages(candidates),
		TokenBudget:     tokenBudget,
	}

	tpl, err := template.ParseFS(compactionPromptFS, "template/compaction.md.tpl")
	if err != nil {
		return "", fmt.Errorf("parse compaction prompt: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render compaction prompt: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func (r *QueryLoop) compactContext(ctx context.Context, request compactionRequest) (compactionResult, error) {
	if strings.TrimSpace(request.Model) == "" {
		return compactionResult{}, fmt.Errorf("agent: compaction model is required")
	}
	if request.TokenBudget <= 0 {
		return compactionResult{}, fmt.Errorf("agent: compaction token budget must be greater than 0")
	}

	source := append(cloneMessages(request.Candidates), request.RecentContext...)
	if strings.TrimSpace(request.ExistingSummary) != "" {
		source = append([]messagecontract.Message{{
			Kind: messagecontract.KindSystem,
			Parts: []messagecontract.Part{{
				Type: messagecontract.PartTypeSummary,
				Text: request.ExistingSummary,
			}},
		}}, source...)
	}
	fallback := buildCompactSummaryMessage("", source).Parts[0].Text
	summaryText, err := truncateTextToTokenBudget(request.Model, fallback, request.TokenBudget)
	if err != nil {
		return compactionResult{}, err
	}

	if r.deps.Provider != nil {
		prompt, renderErr := r.renderCompactionPrompt(
			request.CurrentTask,
			request.ExistingSummary,
			request.RecentContext,
			request.Candidates,
			request.TokenBudget,
		)
		if renderErr == nil {
			result, providerErr := r.deps.Provider.RunTurn(ctx, providercontract.Request{
				Messages: []messagecontract.Message{{
					Kind:  messagecontract.KindUser,
					Parts: []messagecontract.Part{{Type: messagecontract.PartTypeText, Text: prompt}},
				}},
			})
			providerSummary := strings.TrimSpace(result.Text)
			if providerErr == nil && validateCompactionSummary(providerSummary) == nil {
				summaryText, err = truncateTextToTokenBudget(request.Model, providerSummary, request.TokenBudget)
				if err != nil {
					return compactionResult{}, err
				}
			}
		}
	}

	summaryTokens, err := countTokens(request.Model, summaryText)
	if err != nil {
		return compactionResult{}, err
	}
	sourceIDs := make([]string, 0, len(request.Candidates))
	for _, msg := range request.Candidates {
		if msg.ID != "" {
			sourceIDs = append(sourceIDs, msg.ID)
		}
	}
	return compactionResult{
		SummaryText: summaryText,
		SourceIDs:   sourceIDs,
		TokenCount:  summaryTokens,
	}, nil
}

func validateCompactionSummary(summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return fmt.Errorf("agent: compaction provider returned an empty summary")
	}
	for _, section := range []string{"# Task", "## Goal", "## Verification", "## Open Items"} {
		if !strings.Contains(summary, section) {
			return fmt.Errorf("agent: compaction summary is missing %q", section)
		}
	}
	return nil
}

func formatPromptMessages(messages []messagecontract.Message) []PromptMessage {
	formatted := make([]PromptMessage, 0, len(messages))
	for _, msg := range messages {
		content := formatCompactionMessage(msg)
		if strings.TrimSpace(content) == "" {
			continue
		}
		formatted = append(formatted, PromptMessage{
			Role:    string(msg.Kind),
			Content: content,
		})
	}
	return formatted
}

func formatCompactionMessage(msg messagecontract.Message) string {
	var content strings.Builder
	for _, part := range msg.Parts {
		switch part.Type {
		case messagecontract.PartTypeText:
			content.WriteString(part.Text)
		case messagecontract.PartTypeThinking:
			if part.Thinking != nil {
				content.WriteString("\n[thinking]\n")
				content.WriteString(part.Thinking.Content)
			}
		case messagecontract.PartTypeToolCall:
			if part.ToolCall != nil {
				content.WriteString("\n[tool call: ")
				content.WriteString(part.ToolCall.Name)
				content.WriteString("]\n")
				content.WriteString(part.ToolCall.Input)
			}
		case messagecontract.PartTypeToolResult:
			if part.ToolResult != nil {
				content.WriteString("\n[tool result]\n")
				content.WriteString(part.ToolResult.Content)
			}
		case messagecontract.PartTypeSummary:
			content.WriteString(part.Text)
		}
	}
	return strings.TrimSpace(content.String())
}
