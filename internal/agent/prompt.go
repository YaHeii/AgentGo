package agent

import (
	"bytes"
	"context"
	"embed"
	"strings"
	"text/template"

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
	History     []PromptMessage
	UserInput   string
}

type PromptMessage struct {
	Role    string
	Content string
}

func (r *QueryLoop) renderPrompt(state LoopState, usrPrompt string) (string, error) {
	runtimeState := r.runtimeSnapshot()
	requestMessages := trimPendingAssistant(state.Messages)
	var tools []toolcontract.Metadata
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), runtimeState.PermissionLevel)
	}
	data := promptTemplateData{
		AppVersion:  runtimeState.AppVersion,
		ProjectRoot: runtimeState.ProjectRoot,
		Cwd:         runtimeState.Cwd,
		Tools:       tools,
		History:     buildPromptHistory(requestMessages),
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

func (r *QueryLoop) buildInitialRequest(state LoopState, prompt string) (providercontract.Request, error) {
	runtimeState := r.runtimeSnapshot()
	requestMessages := trimPendingAssistant(state.Messages)
	var tools []toolcontract.Metadata
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), runtimeState.PermissionLevel)
	}
	req := providercontract.Request{
		Messages: []message.Message{
			{
				Kind: message.KindSystem,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: prompt},
				},
			},
		},
		Tools: make([]toolcontract.Metadata, 0, len(tools)),
	}
	if runtimeState.Temperature != 0 {
		temperature := runtimeState.Temperature
		req.Context.Temperature = &temperature
	}
	if runtimeState.ModelLimit > 0 {
		maxOutputTokens := runtimeState.ModelLimit
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

func (r *QueryLoop) renderLoopstate(state LoopState) (providercontract.Request, error) {
	runtimeState := r.runtimeSnapshot()
	requestMessages := trimPendingAssistant(state.Messages)
	var tools []toolcontract.Metadata
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), runtimeState.PermissionLevel)
	}

	req := providercontract.Request{
		Messages: make([]message.Message, 0, len(requestMessages)),
		Tools:    make([]toolcontract.Metadata, 0, len(tools)),
	}
	if runtimeState.Temperature != 0 {
		temperature := runtimeState.Temperature
		req.Context.Temperature = &temperature
	}
	if runtimeState.ModelLimit > 0 {
		maxOutputTokens := runtimeState.ModelLimit
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
