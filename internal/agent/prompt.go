package agent

import (
	"bytes"
	"embed"
	"strings"
	"text/template"

	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/provider"
)

//go:embed template/prompt.md.tpl
var promptFS embed.FS

type promptTemplateData struct {
	AppVersion  string
	ProjectRoot string
	Cwd         string
	Tools       []lifecycle.ToolSnapshot
	History     []PromptMessage
	UserInput   string
}

type PromptMessage struct {
	Role    string
	Content string
}

func (r *QueryLoop) renderLoopstate(state LoopState) (provider.Request, error) {
	runtimeState := lifecycle.GetState()
	requestMessages := trimPendingAssistant(state.Messages)
	tools := availableToolSnapshots(runtimeState)
	data := promptTemplateData{
		AppVersion:  runtimeState.AppVersion,
		ProjectRoot: runtimeState.ProjectRoot,
		Cwd:         runtimeState.Cwd,
		Tools:       tools,
		History:     buildPromptHistory(requestMessages),
		UserInput:   latestUserInput(requestMessages),
	}

	tpl, err := template.ParseFS(promptFS, "template/prompt.md.tpl")
	if err != nil {
		return provider.Request{}, err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return provider.Request{}, err
	}

	req := provider.Request{
		Messages: []provider.Message{
			{
				Role:    provider.RoleSystem,
				Content: strings.TrimSpace(buf.String()),
			},
		},
		Tools: make([]provider.ToolDefinition, 0, len(tools)),
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
		req.Tools = append(req.Tools, provider.ToolDefinition{
			Name:        meta.Name,
			Description: meta.Description,
			Parameters:  meta.Parameters,
		})
	}
	return req, nil
}

func availableToolSnapshots(state lifecycle.GlobalState) []lifecycle.ToolSnapshot {
	if len(state.KnownTools) == 0 {
		return nil
	}

	level := int(state.PermissionLevel)
	tools := make([]lifecycle.ToolSnapshot, 0, len(state.KnownTools))
	for _, meta := range state.KnownTools {
		if !meta.Enabled {
			continue
		}
		if level < meta.SecurityLevel {
			continue
		}
		tools = append(tools, meta)
	}
	return tools
}
