package agent

import (
	"bytes"
	"context"
	"embed"
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
	History     []PromptMessage
	UserInput   string
}

type PromptMessage struct {
	Role    string
	Content string
}

func (r *QueryLoop) renderPrompt(state LoopState, usrPrompt string) (string, error) {
	requestMessages := trimPendingAssistant(state.Messages)
	var tools []toolcontract.Metadata
	permissionLevel := toolcontract.SecurityLevel(0)
	if lifecycle.State != nil {
		permissionLevel = toolcontract.SecurityLevel(lifecycle.State.PermissionLevel)
	}
	if r.deps.App != nil {
		tools = r.deps.App.ListTools(context.Background(), permissionLevel)
	}
	appVersion := ""
	projectRoot := ""
	cwd := ""
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

func (r *QueryLoop) renderLoopstate(state LoopState) (providercontract.Request, error) {
	return r.buildRequest(state), nil
}

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
