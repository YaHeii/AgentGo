package model

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	messageevent "github.com/YaHeii/agentGo/internal/message"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/ui/common"
	"github.com/charmbracelet/bubbles/viewport"
	uv "github.com/charmbracelet/ultraviolet"
)

type markdownRenderer interface {
	Render(string) (string, error)
}

type transcriptItem struct {
	ID        string
	MessageID string
	Kind      message.Kind
	PartType  message.PartType
	Summary   string
	Body      string
	Expanded  bool
	Final     bool
}

type transcriptModel struct {
	viewport viewport.Model
	items    []transcriptItem
	selected int
	expanded map[string]bool
}

func newTranscriptModel(width int, height int) transcriptModel {
	vp := viewport.New(width, height)
	return transcriptModel{
		viewport: vp,
		expanded: make(map[string]bool),
	}
}

func (m transcriptModel) updateSize(width int, height int) transcriptModel {
	m.viewport.Width = max(1, width)
	m.viewport.Height = max(1, height)
	m.ensureSelectionVisible()
	return m
}

func (m transcriptModel) setItems(items []transcriptItem) transcriptModel {
	m.items = append([]transcriptItem(nil), items...)
	if m.expanded == nil {
		m.expanded = make(map[string]bool)
	}
	if len(m.items) == 0 {
		m.selected = 0
	} else if m.selected >= len(m.items) {
		m.selected = len(m.items) - 1
	}
	m.syncViewport()
	return m
}

func (m transcriptModel) Update(msg tea.Msg) transcriptModel {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m
	}

	switch keyMsg.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
			m.ensureSelectionVisible()
		}
	case "down", "j":
		if m.selected < len(m.items)-1 {
			m.selected++
			m.ensureSelectionVisible()
		}
	case "enter", " ", "space":
		if len(m.items) == 0 {
			return m
		}
		item := m.items[m.selected]
		if !itemCanExpand(item) {
			return m
		}
		m.expanded[item.ID] = !m.expanded[item.ID]
		m.syncViewport()
	case "pgdown":
		m.viewport.PageDown()
	case "pgup":
		m.viewport.PageUp()
	}

	return m
}

func (m transcriptModel) View() string {
	return m.viewport.View()
}

func (m transcriptModel) selectedItem() (transcriptItem, bool) {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		return transcriptItem{}, false
	}
	return m.items[m.selected], true
}

func (m *transcriptModel) syncViewport() {
	contentLines := make([]string, 0, len(m.items)*2)
	for i, item := range m.items {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}

		contentLines = append(contentLines, prefix+item.Summary)
		if m.expanded[item.ID] && strings.TrimSpace(item.Body) != "" {
			for _, line := range strings.Split(item.Body, "\n") {
				contentLines = append(contentLines, "    "+line)
			}
		}
	}
	m.viewport.SetContent(strings.Join(contentLines, "\n"))
}

func (m *transcriptModel) ensureSelectionVisible() {
	if len(m.items) == 0 {
		return
	}
	m.syncViewport()
	if m.selected < m.viewport.YOffset {
		m.viewport.SetYOffset(m.selected)
		return
	}
	bottom := m.viewport.YOffset + max(1, m.viewport.Height) - 1
	if m.selected > bottom {
		m.viewport.SetYOffset(m.selected - max(1, m.viewport.Height) + 1)
	}
}

type Chat struct {
	app           appService
	events        <-chan app.Event
	sessionID     string
	messages      []message.Message
	items         []transcriptItem
	markdown      map[string]string
	quietMarkdown map[string]string
	renderer      markdownRenderer
	quietRenderer markdownRenderer
	errMessage    string
	loading       bool
	transcript    transcriptModel
	composer      composerModel
	status        statusModel
}

func newChat(appSvc appService) Chat {
	return Chat{
		app:           appSvc,
		events:        appSvc.Events(),
		markdown:      make(map[string]string),
		quietMarkdown: make(map[string]string),
		renderer:      common.MarkdownRenderer(defaultWidth),
		quietRenderer: common.QuietMarkdownRenderer(defaultWidth),
		transcript:    newTranscriptModel(defaultWidth, defaultHeight),
		composer:      newComposerModel(),
		status:        newStatusModel(),
	}
}

func (c *Chat) Draw(scr uv.Screen, area uv.Rectangle) {
	c.transcript = c.transcript.updateSize(area.Dx(), area.Dy())
	uv.NewStyledString(c.transcript.View()).Draw(scr, area)
}

func (c *Chat) DrawStatus(scr uv.Screen, area uv.Rectangle) {
	statusText := c.status.View()
	if statusText == "" {
		statusText = "ready"
	}
	uv.NewStyledString(statusText).Draw(scr, area)
}

func (c *Chat) DrawEditor(scr uv.Screen, area uv.Rectangle) {
	uv.NewStyledString(c.composer.View()).Draw(scr, area)
}

func (c *Chat) updateSize(width int) {
	c.renderer = common.MarkdownRenderer(max(1, width))
	c.quietRenderer = common.QuietMarkdownRenderer(max(1, width))
}

func bootstrapSessionCmd(appSvc appService) tea.Cmd {
	return func() tea.Msg {
		err := appSvc.EnsureActiveSession(context.Background())
		return bootstrapDoneMsg{err: err}
	}
}

func runQueryCmd(appSvc appService, sessionID string, prompt string) tea.Cmd {
	return func() tea.Msg {
		err := appSvc.RunQuery(context.Background(), sessionID, prompt)
		return sendMessageDoneMsg{err: err}
	}
}

func loadHistoryCmd(appSvc appService, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := appSvc.ListHistory(context.Background(), sessionID)
		return historyLoadedMsg{
			sessionID: sessionID,
			messages:  messages,
			err:       err,
		}
	}
}

func waitAppEventCmd(events <-chan app.Event) tea.Cmd {
	return func() tea.Msg {
		event := <-events
		return appEventMsg{event: event}
	}
}

func (c *Chat) upsertMessage(msg message.Message) {
	for i := range c.messages {
		if c.messages[i].ID == msg.ID {
			c.messages[i] = msg
			return
		}
	}

	c.messages = append(c.messages, msg)
}

func (c *Chat) refreshTranscript() {
	c.items = buildTranscriptItems(c.messages, c.markdown, c.quietMarkdown, c.transcript.expanded)
	c.transcript = c.transcript.setItems(c.items)
}

func (c *Chat) clearMarkdownForMessages(messages []message.Message) {
	for _, msg := range messages {
		delete(c.markdown, msg.ID)
		delete(c.quietMarkdown, msg.ID)
	}
}

func (c *Chat) renderStableMarkdown(messages []message.Message) {
	for _, msg := range messages {
		c.renderMessageMarkdown(msg)
	}
}

func (c *Chat) renderTerminalMarkdown(messages []message.Message) {
	c.renderStableMarkdown(messages)
}

func (c *Chat) renderMessageMarkdown(msg message.Message) {
	if text := textContent(msg.Parts); strings.TrimSpace(text) != "" {
		if rendered, ok := renderMarkdown(c.renderer, text); ok {
			c.markdown[msg.ID] = rendered
		}
	}

	if msg.Kind != message.KindAssistant {
		return
	}

	detailParts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		switch part.Type {
		case message.PartTypeThinking:
			if part.Thinking == nil {
				continue
			}
			body := normalizeDisplayText(part.Thinking.Content)
			if body == "" {
				body = normalizeDisplayText(part.Thinking.Summary)
			}
			if body != "" {
				if rendered, ok := renderMarkdown(c.quietRenderer, body); ok {
					detailParts = append(detailParts, "thinking", rendered)
				}
			}
		case message.PartTypeToolCall:
			if part.ToolCall == nil {
				continue
			}
			detailParts = append(detailParts, "tool call: "+part.ToolCall.Name)
			if body := normalizeDisplayText(part.ToolCall.Input); body != "" {
				if rendered, ok := renderMarkdown(c.quietRenderer, body); ok {
					detailParts = append(detailParts, rendered)
				} else {
					detailParts = append(detailParts, body)
				}
			}
		case message.PartTypeToolResult:
			if part.ToolResult == nil {
				continue
			}
			detailParts = append(detailParts, "tool result: "+part.ToolResult.ToolCallID)
			if body := normalizeDisplayText(part.ToolResult.Content); body != "" {
				if rendered, ok := renderMarkdown(c.quietRenderer, body); ok {
					detailParts = append(detailParts, rendered)
				} else {
					detailParts = append(detailParts, body)
				}
			}
		}
	}

	if len(detailParts) > 0 {
		c.quietMarkdown[msg.ID] = strings.TrimSpace(strings.Join(detailParts, "\n"))
	}
}

func (c *Chat) handleSessionEvent(event session.SessionEvent) tea.Cmd {
	if event.Session == nil {
		return nil
	}
	c.sessionID = event.Session.ID
	if event.Status == session.StatusRestored || event.Status == session.StatusSwitched {
		return loadHistoryCmd(c.app, event.Session.ID)
	}
	return nil
}

func (c *Chat) handleMessageEvent(event messageevent.MessageEvent) {
	if event.Message == nil {
		return
	}
	c.upsertMessage(*event.Message)
	c.refreshTranscript()
}

func (c *Chat) handleAgentEvent(event agent.QueryEvent) {
	c.messages = append([]message.Message(nil), event.State.Messages...)

	switch event.Status {
	case agent.QueryStatusStarted, agent.QueryStatusDelta:
		c.clearMarkdownForMessages(c.messages)
		c.loading = true
		c.status = c.status.setMessage("assistant is thinking...")
	case agent.QueryStatusCompleted:
		c.renderTerminalMarkdown(c.messages)
		c.loading = false
		c.status = c.status.setMessage("")
	case agent.QueryStatusFailed:
		c.renderTerminalMarkdown(c.messages)
		if event.Err != nil {
			c.errMessage = event.Err.Error()
			c.status = c.status.setMessage(c.errMessage)
		}
		c.loading = false
	}

	c.refreshTranscript()
}

func buildTranscriptItems(messages []message.Message, markdown map[string]string, quietMarkdown map[string]string, expanded map[string]bool) []transcriptItem {
	items := make([]transcriptItem, 0, len(messages))
	for _, msg := range messages {
		if msg.Kind == message.KindAssistant {
			item, ok := buildAssistantTranscriptItem(msg, markdown, quietMarkdown)
			if ok {
				if expanded != nil {
					item.Expanded = expanded[item.ID]
				}
				items = append(items, item)
			}
			continue
		}

		for partIndex, part := range msg.Parts {
			item, ok := buildPartTranscriptItem(msg, partIndex, part, markdown, quietMarkdown)
			if ok {
				if expanded != nil {
					item.Expanded = expanded[item.ID]
				}
				items = append(items, item)
			}
		}

		if msg.System != nil {
			item := transcriptItem{
				ID:        fmt.Sprintf("%s:system", msg.ID),
				MessageID: msg.ID,
				Kind:      msg.Kind,
				Summary:   buildSystemSummary(msg.System),
				Body:      textContent(msg.Parts),
				Final:     true,
			}
			if expanded != nil {
				item.Expanded = expanded[item.ID]
			}
			items = append(items, item)
		}

		if msg.Progress != nil {
			item := transcriptItem{
				ID:        fmt.Sprintf("%s:progress", msg.ID),
				MessageID: msg.ID,
				Kind:      msg.Kind,
				Summary:   buildProgressSummary(msg.Progress),
				Body:      textContent(msg.Parts),
				Final:     true,
			}
			if expanded != nil {
				item.Expanded = expanded[item.ID]
			}
			items = append(items, item)
		}
	}

	return items
}

func buildAssistantTranscriptItem(msg message.Message, markdown map[string]string, quietMarkdown map[string]string) (transcriptItem, bool) {
	textBody := ""
	detailLines := make([]string, 0, len(msg.Parts))
	summaryParts := make([]string, 0, len(msg.Parts))
	hasRenderedDetails := false

	if renderedDetails, ok := quietMarkdown[msg.ID]; ok && renderedDetails != "" {
		detailLines = append(detailLines, renderedDetails)
		hasRenderedDetails = true
	}

	for _, part := range msg.Parts {
		switch part.Type {
		case message.PartTypeText:
			body := normalizeDisplayText(part.Text)
			if rendered, ok := markdown[msg.ID]; ok && rendered != "" {
				body = normalizeDisplayText(rendered)
			}
			if body != "" {
				textBody = body
			}
		case message.PartTypeThinking:
			summaryParts = append(summaryParts, "thinking")
			if part.Thinking != nil && !hasRenderedDetails {
				body := normalizeDisplayText(part.Thinking.Content)
				if body == "" {
					body = normalizeDisplayText(part.Thinking.Summary)
				}
				if body != "" {
					detailLines = append(detailLines, "thinking", body)
				}
			}
		case message.PartTypeToolCall:
			if part.ToolCall == nil {
				continue
			}
			summaryParts = append(summaryParts, fmt.Sprintf("tool call: %s", part.ToolCall.Name))
			if hasRenderedDetails {
				continue
			}
			detailLines = append(detailLines, fmt.Sprintf("tool call: %s", part.ToolCall.Name))
			if body := normalizeDisplayText(part.ToolCall.Input); body != "" {
				detailLines = append(detailLines, body)
			}
		case message.PartTypeToolResult:
			if part.ToolResult == nil {
				continue
			}
			summaryParts = append(summaryParts, fmt.Sprintf("tool result: %s", part.ToolResult.ToolCallID))
			if hasRenderedDetails {
				continue
			}
			detailLines = append(detailLines, fmt.Sprintf("tool result: %s", part.ToolResult.ToolCallID))
			if body := normalizeDisplayText(part.ToolResult.Content); body != "" {
				detailLines = append(detailLines, body)
			}
		}
	}

	item := transcriptItem{
		ID:        msg.ID,
		MessageID: msg.ID,
		Kind:      msg.Kind,
		PartType:  message.PartTypeText,
		Final:     textBody != "",
	}

	if textBody != "" {
		item.Summary = fmt.Sprintf("%s %s", kindLabel(msg.Kind), summarizeText(textBody))
		item.Body = strings.TrimSpace(strings.Join(append([]string{textBody}, detailLines...), "\n"))
		return item, true
	}

	if len(summaryParts) == 0 {
		return transcriptItem{}, false
	}

	item.Summary = fmt.Sprintf("%s %s", kindLabel(msg.Kind), strings.Join(summaryParts, " / "))
	item.Body = strings.TrimSpace(strings.Join(detailLines, "\n"))
	return item, true
}

func buildPartTranscriptItem(msg message.Message, partIndex int, part message.Part, markdown map[string]string, quietMarkdown map[string]string) (transcriptItem, bool) {
	item := transcriptItem{
		ID:        fmt.Sprintf("%s:%d:%s", msg.ID, partIndex, part.Type),
		MessageID: msg.ID,
		Kind:      msg.Kind,
		PartType:  part.Type,
	}

	switch part.Type {
	case message.PartTypeText:
		if msg.Kind == message.KindSystem && (msg.System != nil || msg.Progress != nil) {
			return transcriptItem{}, false
		}
		body := normalizeDisplayText(part.Text)
		if rendered, ok := markdown[msg.ID]; ok && rendered != "" {
			body = normalizeDisplayText(rendered)
		}
		item.Summary = fmt.Sprintf("%s %s", kindLabel(msg.Kind), summarizeText(body))
		item.Body = body
		item.Final = true
		return item, true
	case message.PartTypeThinking:
		item.Summary = "thinking"
		if rendered, ok := quietMarkdown[msg.ID]; ok && rendered != "" {
			item.Body = rendered
			return item, true
		}
		if part.Thinking != nil {
			item.Body = strings.TrimSpace(part.Thinking.Content)
			if item.Body == "" {
				item.Body = strings.TrimSpace(part.Thinking.Summary)
			}
		}
		return item, true
	case message.PartTypeToolCall:
		if part.ToolCall == nil {
			return transcriptItem{}, false
		}
		item.Summary = fmt.Sprintf("tool call: %s", part.ToolCall.Name)
		if rendered, ok := quietMarkdown[msg.ID]; ok && rendered != "" {
			item.Body = rendered
			return item, true
		}
		item.Body = strings.TrimSpace(part.ToolCall.Input)
		return item, true
	case message.PartTypeToolResult:
		if part.ToolResult == nil {
			return transcriptItem{}, false
		}
		item.Summary = fmt.Sprintf("tool result: %s", part.ToolResult.ToolCallID)
		if rendered, ok := quietMarkdown[msg.ID]; ok && rendered != "" {
			item.Body = rendered
			return item, true
		}
		item.Body = strings.TrimSpace(part.ToolResult.Content)
		return item, true
	}

	return transcriptItem{}, false
}

func summarizeText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	return strings.TrimSpace(lines[0])
}

func buildSystemSummary(payload *message.SystemPayload) string {
	if payload == nil {
		return "system"
	}
	if strings.TrimSpace(payload.Subtype) != "" {
		return fmt.Sprintf("system: %s", payload.Subtype)
	}
	if strings.TrimSpace(payload.Level) != "" {
		return fmt.Sprintf("system: %s", payload.Level)
	}
	return "system"
}

func buildProgressSummary(payload *message.ProgressPayload) string {
	if payload == nil {
		return "progress"
	}
	stage := strings.TrimSpace(payload.Stage)
	if stage == "" {
		stage = "running"
	}
	if payload.Total > 0 {
		return fmt.Sprintf("progress: %s %d/%d", stage, payload.Current, payload.Total)
	}
	return fmt.Sprintf("progress: %s", stage)
}

func normalizeDisplayText(text string) string {
	normalized := strings.ReplaceAll(text, "\\n", "\n")
	return strings.TrimSpace(normalized)
}

func itemCanExpand(item transcriptItem) bool {
	if strings.TrimSpace(item.Body) == "" {
		return false
	}
	if item.Kind == message.KindAssistant {
		return true
	}
	return item.PartType != message.PartTypeText
}

func renderMarkdown(renderer markdownRenderer, input string) (string, bool) {
	if renderer == nil || strings.TrimSpace(input) == "" {
		return "", false
	}
	rendered, err := renderer.Render(input)
	if err != nil {
		return "", false
	}
	return rendered, true
}

func kindLabel(kind message.Kind) string {
	switch kind {
	case message.KindAssistant:
		return "ai:"
	default:
		return "you:"
	}
}

func textContent(parts []message.Part) string {
	for _, part := range parts {
		if part.Type == message.PartTypeText {
			return part.Text
		}
	}
	return ""
}
