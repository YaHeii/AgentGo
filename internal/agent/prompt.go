package agent

import (
	"bytes"
	"context"
	"embed"
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
	req := providercontract.Request{
		Messages: []message.Message{
			{
				Kind: message.KindSystem,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: prompt},
				},
			},
		},
	}
	loopReq := r.buildRequest(state)
	req.Messages = append(req.Messages, loopReq.Messages...)
	req.Tools = loopReq.Tools
	req.Context = loopReq.Context
	return req, nil
}

// buildRequest replays persisted conversation state into the provider format
// and attaches the currently allowed tool metadata.
func (r *QueryLoop) buildRequest(state LoopState) providercontract.Request {
	requestMessages := trimPendingAssistant(state.Messages)
	permissionLevel := toolcontract.SecurityLevel(0)
	if lifecycle.State != nil {
		permissionLevel = toolcontract.SecurityLevel(lifecycle.State.PermissionLevel)
	}
	var tools []toolcontract.Metadata
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), permissionLevel)
	}

	req := providercontract.Request{
		Messages: make([]message.Message, 0, len(requestMessages)),
		Tools:    make([]toolcontract.Metadata, 0, len(tools)),
	}
	if lifecycle.State != nil && lifecycle.State.Temperature != 0 {
		temperature := lifecycle.State.Temperature
		req.Context.Temperature = &temperature
	}
	if lifecycle.State != nil && lifecycle.State.ModelLimit > 0 {
		maxOutputTokens := lifecycle.State.ModelLimit
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
	return req
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
