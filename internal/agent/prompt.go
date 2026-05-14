package agent

import (
	"bytes"
	"embed"
	"strings"
	"text/template"

	"github.com/YaHeii/agentGo/internal/tool"
)

//go:embed template/prompt.md.tpl
var promptFS embed.FS

type PromptContext struct {
	AppVersion   string
	ProjectRoot  string
	Cwd          string
	Tools        []tool.Metadata
	History      []PromptMessage
	UserInput    string
	Instructions []string
}

type PromptMessage struct {
	Role    string
	Content string
}

func (r *QueryLoop) renderPrompt(data PromptContext) (string, error) {
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
