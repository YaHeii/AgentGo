package agent

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/YaHeii/agentGo/internal/lifecycle"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

//go:embed template/prompt.md.tpl
var promptFS embed.FS

type promptTemplateData struct {
	AppVersion  string
	ProjectRoot string
	Cwd         string
	Tools       []toolcontract.Metadata
	AGENTSMD    string
	History     []PromptMessage
	UserInput   string
}

type PromptMessage struct {
	Role    string
	Content string
}

// renderPrompt builds the initial system prompt that wraps runtime context,
// available tools, prior history, and the latest user input.
func (r *QueryLoop) renderPrompt(usrPrompt string) (string, error) {
	var tools []toolcontract.Metadata
	permissionLevel := toolcontract.SecurityLevel(0)
	if lifecycle.State != nil {
		permissionLevel = toolcontract.SecurityLevel(lifecycle.State.PermissionLevel)
	}
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), permissionLevel)
	}

	agentsMD, err := LoadAggregateInstructions()

	appVersion := lifecycle.State.AppVersion
	projectRoot := lifecycle.State.ProjectRoot
	cwd := lifecycle.State.Cwd

	if lifecycle.State != nil {
		appVersion = lifecycle.State.AppVersion
		projectRoot = lifecycle.State.ProjectRoot
		cwd = lifecycle.State.Cwd
	}
	data := promptTemplateData{
		AppVersion:  appVersion,
		ProjectRoot: projectRoot,
		Cwd:         cwd,
		Tools:       tools,
		AGENTSMD:    agentsMD,
		UserInput:   usrPrompt,
	}

	tpl, err := template.ParseFS(promptFS, "template/prompt.md.tpl")
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

// Load AGENTS.md
func LoadAggregateInstructions() (string, error) {
	var sb strings.Builder

	// homelevel
	homeDir, err := os.UserHomeDir()
	if err != nil {

		homeDir = ""
	}

	// Level 1: Managed Memory (System-wide Hardcoded Constraint — Lowest Priority)
	appendFileIfExists(&sb, "/etc/agentgo/AGENTS.md", "Managed Global Rules")

	// Level 2: User Memory (User-Specific Private Constraints Across Projects)
	if homeDir != "" {
		userPath := filepath.Join(homeDir, ".agents", "AGENTS.md")
		appendFileIfExists(&sb, userPath, "User Private Rules")
	}

	// Level 3: Project Memory (Constraints Shared within the Version Control Repository)
	projectRoot := lifecycle.State.ProjectRoot
	if projectRoot != "" {
		appendFileIfExists(&sb, filepath.Join(projectRoot, "AGENTS.md"), "Project Core Rules")

		appendFileIfExists(&sb, filepath.Join(projectRoot, ".agents", "AGENTS.md"), "Project Specific Rules")

		rulesPattern := filepath.Join(projectRoot, ".agents", "rules", "*.md")
		appendGlobFilesIfExists(&sb, rulesPattern)
	}

	// Level 4: Local Memory (Private Overrides — Not Committed — Highest Priority)
	if projectRoot != "" {
		localPath := filepath.Join(projectRoot, "AGENTS.local.md")
		appendFileIfExists(&sb, localPath, "Local Overrides")
	}

	return sb.String(), nil
}

func appendFileIfExists(sb *strings.Builder, path string, sectionName string) {
	content, err := os.ReadFile(path)
	if err != nil {
		// Whether the file does not exist (os.IsNotExist) or there are insufficient permissions to read it,
		//  both are treated here as valid branches to be ignored.
		return
	}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return
	}

	sb.WriteString(fmt.Sprintf("\n## %s (%s)\n", sectionName, path))
	sb.WriteString(trimmed)
	sb.WriteString("\n")
}

func appendGlobFilesIfExists(sb *strings.Builder, pattern string) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	for _, path := range matches {
		filename := filepath.Base(path)
		sectionName := fmt.Sprintf("Project Rule: %s", filename)
		appendFileIfExists(sb, path, sectionName)
	}
}

// buildInitialRequest prepends the rendered system prompt to the normal
// provider request used for the first turn.
func (r *QueryLoop) buildInitialRequest(state LoopState, prompt string) (providercontract.Request, error) {
	loopReq, err := r.buildRequestWithSystemPrompt(state, prompt)
	if err != nil {
		return providercontract.Request{}, err
	}
	req := providercontract.Request{
		Messages: []message.Message{{
			Kind:  message.KindSystem,
			Parts: []message.Part{{Type: message.PartTypeText, Text: prompt}},
		}},
	}
	req.Messages = append(req.Messages, loopReq.Messages...)
	req.Tools = loopReq.Tools
	req.Context = loopReq.Context
	return req, nil
}

// buildRequest replays persisted conversation state into the provider format
// and attaches the currently allowed tool metadata.
func (r *QueryLoop) buildRequest(state LoopState) (providercontract.Request, error) {
	return r.buildRequestWithSystemPrompt(state, "")
}

func (r *QueryLoop) buildRequestWithSystemPrompt(state LoopState, systemPrompt string) (providercontract.Request, error) {
	requestMessages := trimPendingAssistant(state.Messages)
	permissionLevel := toolcontract.SecurityLevel(0)
	if lifecycle.State != nil {
		permissionLevel = toolcontract.SecurityLevel(lifecycle.State.PermissionLevel)
	}
	var tools []toolcontract.Metadata
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), permissionLevel)
	}

	model := ""
	modelLimit := 0
	outputTokens := 0
	if lifecycle.State != nil {
		model = lifecycle.State.Model
		modelLimit = lifecycle.State.ModelLimit
		outputTokens = lifecycle.State.MaxOutputTokens
	}
	if strings.TrimSpace(model) == "" || modelLimit <= 0 {
		return providercontract.Request{}, errors.New("agent: model and context window are required")
	}
	budget, err := calculateContextBudget(model, modelLimit, outputTokens, systemPrompt, tools)
	if err != nil {
		return providercontract.Request{}, err
	}
	requestMessages, err = selectHistoryByTokenBudget(model, requestMessages, budget.HistoryBudget)
	if err != nil {
		return providercontract.Request{}, err
	}

	req := providercontract.Request{
		Messages: make([]message.Message, 0, len(requestMessages)),
		Tools:    make([]toolcontract.Metadata, 0, len(tools)),
	}
	if lifecycle.State != nil && lifecycle.State.Temperature != 0 {
		temperature := lifecycle.State.Temperature
		req.Context.Temperature = &temperature
	}
	if lifecycle.State != nil && lifecycle.State.MaxOutputTokens > 0 {
		maxOutputTokens := lifecycle.State.MaxOutputTokens
		req.Context.MaxOutputTokens = &maxOutputTokens
	}
	for _, msg := range requestMessages {
		providerMsg, ok := toProviderMessage(msg)
		if !ok {
			continue
		}
		req.Messages = append(req.Messages, providerMsg)
	}
	for _, meta := range tools {
		req.Tools = append(req.Tools, meta)
	}
	return req, nil
}

func (r *QueryLoop) prepareHistory(ctx context.Context, sessionID string, history []message.Message, systemPrompt string) ([]message.Message, error) {
	processed := cloneMessages(history)
	if r.deps.App == nil {
		return processed, nil
	}

	permissionLevel := toolcontract.SecurityLevel(0)
	if lifecycle.State != nil {
		permissionLevel = toolcontract.SecurityLevel(lifecycle.State.PermissionLevel)
	}
	tools := r.deps.App.ListTools(ctx, permissionLevel)
	if lifecycle.State == nil || strings.TrimSpace(lifecycle.State.Model) == "" || lifecycle.State.ModelLimit <= 0 {
		return processed, nil
	}

	model := lifecycle.State.Model
	budget, err := calculateContextBudget(model, lifecycle.State.ModelLimit, 0, systemPrompt, tools)
	if err != nil {
		return nil, err
	}
	active := historyAfterLatestSummary(processed)
	historyTokens, err := countMessagesTokens(model, active)
	if err != nil {
		return nil, err
	}
	if budget.ShouldCompact(budget.FixedTokens + historyTokens) {
		active, err = r.compactHistory(ctx, sessionID, active, model, budget, systemPrompt)
		if err != nil {
			return nil, err
		}
	}
	return selectHistoryByTokenBudget(model, active, budget.HistoryBudget)
}

func (r *QueryLoop) compactHistory(ctx context.Context, sessionID string, history []message.Message, model string, budget contextBudget, currentTask string) ([]message.Message, error) {
	if len(history) == 0 {
		return history, nil
	}
	target := budget.ModelLimit*compactedHistoryPercent/100 - budget.FixedTokens
	if target < 1 {
		target = 1
	}
	kept, err := selectHistoryByTokenBudget(model, history, target)
	if err != nil {
		return nil, err
	}
	keptIDs := make(map[string]struct{}, len(kept))
	for _, msg := range kept {
		keptIDs[msg.ID] = struct{}{}
	}
	dropped := make([]message.Message, 0, len(history)-len(kept))
	for _, msg := range history {
		if _, ok := keptIDs[msg.ID]; !ok {
			dropped = append(dropped, msg)
		}
	}
	if len(dropped) == 0 {
		return history, nil
	}

	summaryResult, err := r.compactContext(ctx, compactionRequest{
		CurrentTask:     currentTask,
		ExistingSummary: latestSummaryText(history),
		RecentContext:   kept,
		Candidates:      dropped,
		Model:           model,
		TokenBudget:     budget.ModelLimit * compactSummaryPercent / 100,
	})
	if err != nil {
		return nil, err
	}
	summary := buildCompactSummaryMessage(sessionID, nil)
	summaryText := summaryResult.SummaryText
	summary.Parts[0].Text = summaryText
	persisted, err := r.deps.App.CreateMessage(ctx, message.CreateMessageParams{
		SessionID:        summary.SessionID,
		Kind:             summary.Kind,
		IsCompactSummary: summary.IsCompactSummary,
		Parts:            summary.Parts,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: persist compact summary: %w", err)
	}
	return append([]message.Message{persisted}, kept...), nil
}

func historyAfterLatestSummary(history []message.Message) []message.Message {
	lastSummary := -1
	for i, msg := range history {
		if msg.IsCompactSummary {
			lastSummary = i
		}
	}
	if lastSummary < 0 {
		return history
	}
	return history[lastSummary:]
}

func latestSummaryText(history []message.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].IsCompactSummary {
			return strings.TrimSpace(formatCompactionMessage(history[i]))
		}
	}
	return ""
}

// Clean up invalid assistant messages at the end of the message history.
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
