package model

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/YaHeii/agentGo/internal/app"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/ui/common"
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

type Chat struct {
	messages      []message.Message
	items         []transcriptItem
	markdown      map[string]string
	quietMarkdown map[string]string
	renderer      markdownRenderer
	quietRenderer markdownRenderer
	viewport      viewport.Model
	expanded      map[string]bool
	summaryLines  map[int]string
	mainWidth     int
	mainHeight    int
	loading       bool
	autoScroll    bool
}

func newChat() Chat {
	vp := viewport.New(
		viewport.WithWidth(defaultWidth),
		viewport.WithHeight(defaultHeight),
	)
	vp.MouseWheelEnabled = true
	vp.SoftWrap = false

	return Chat{
		markdown:      make(map[string]string),
		quietMarkdown: make(map[string]string),
		renderer:      common.MarkdownRenderer(defaultWidth),
		quietRenderer: common.QuietMarkdownRenderer(defaultWidth),
		viewport:      vp,
		expanded:      make(map[string]bool),
		summaryLines:  make(map[int]string),
		mainWidth:     defaultWidth,
		mainHeight:    defaultHeight,
		autoScroll:    true,
	}
}

func (c *Chat) Draw(scr uv.Screen, area uv.Rectangle) {
	c.Resize(area.Dx(), area.Dy())
	uv.NewStyledString(c.viewport.View()).Draw(scr, area)
}

func (c *Chat) Resize(width int, height int) {
	width = max(1, width)
	height = max(1, height)
	widthChanged := c.mainWidth != width

	c.mainWidth = width
	c.mainHeight = height
	c.viewport.SetWidth(width)
	c.viewport.SetHeight(height)

	if widthChanged {
		c.renderer = common.MarkdownRenderer(width)
		c.quietRenderer = common.QuietMarkdownRenderer(width)
		c.clearMarkdownForMessages(c.messages)
		c.renderTerminalMarkdown(c.messages)
	}

	c.refreshTranscript()
}

func (c *Chat) HandleMouse(msg tea.MouseMsg) {
	switch mouseMsg := msg.(type) {
	case tea.MouseWheelMsg:
		if mouseMsg.Button == tea.MouseWheelUp {
			c.autoScroll = false
		}
		updated, _ := c.viewport.Update(mouseMsg)
		c.viewport = updated
		if mouseMsg.Button == tea.MouseWheelDown {
			c.autoScroll = c.viewport.AtBottom()
		}
	case tea.MouseClickMsg:
		if mouseMsg.Button != tea.MouseLeft {
			return
		}
		lineIndex := c.viewport.YOffset() + mouseMsg.Y
		itemID, ok := c.summaryLines[lineIndex]
		if !ok || itemID == "" {
			return
		}
		c.expanded[itemID] = !c.expanded[itemID]
		c.refreshTranscript()
	}
}

func (c *Chat) SetMessages(messages []message.Message) {
	c.messages = append([]message.Message(nil), messages...)
}

func (c *Chat) syncViewport() {
	width := max(1, c.mainWidth)
	stickToBottom := c.autoScroll || c.viewport.AtBottom()

	lines := make([]string, 0, len(c.items)*3)
	summaryLines := make(map[int]string, len(c.items))

	for _, item := range c.items {
		expandable := itemCanExpand(item)
		summaryPrefix := "  "
		if expandable {
			if c.expanded[item.ID] {
				summaryPrefix = "▾ "
			} else {
				summaryPrefix = "▸ "
			}
		}

		summaryStart := len(lines)
		lines = append(lines, wrapPrefixedText(item.Summary, summaryPrefix, "  ", width)...)
		if expandable {
			for row := summaryStart; row < len(lines); row++ {
				summaryLines[row] = item.ID
			}
		}

		if !c.expanded[item.ID] || strings.TrimSpace(item.Body) == "" {
			continue
		}
		lines = append(lines, wrapPrefixedText(item.Body, "    ", "    ", width)...)
	}

	c.summaryLines = summaryLines
	c.viewport.SetContent(strings.Join(lines, "\n"))
	if stickToBottom {
		c.viewport.GotoBottom()
		c.autoScroll = true
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

func (c *Chat) AppendOrUpsertMessage(msg message.Message) {
	for i := range c.messages {
		if c.messages[i].ID == msg.ID {
			c.messages[i] = msg
			return
		}
	}

	c.messages = append(c.messages, msg)
}

func (c *Chat) ApplyProviderTextDelta(delta string) bool {
	if delta == "" {
		return false
	}

	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Kind != message.KindAssistant {
			continue
		}
		for partIndex := range c.messages[i].Parts {
			if c.messages[i].Parts[partIndex].Type == message.PartTypeText {
				c.messages[i].Parts[partIndex].Text += delta
				return true
			}
		}
		c.messages[i].Parts = append(c.messages[i].Parts, message.Part{
			Type: message.PartTypeText,
			Text: delta,
		})
		return true
	}
	return false
}

func (c *Chat) refreshTranscript() {
	c.items = buildTranscriptItems(c.messages, c.markdown, c.quietMarkdown, c.expanded)
	c.syncViewport()
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
	case message.KindSystem:
		return "system:"
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

func wrapPrefixedText(text string, firstPrefix string, nextPrefix string, width int) []string {
	if width <= 0 {
		width = 1
	}

	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	currentPrefix := firstPrefix

	for _, rawLine := range rawLines {
		availableWidth := max(1, width-lipgloss.Width(currentPrefix))
		wrapped := lipgloss.Wrap(rawLine, availableWidth, "")
		segments := strings.Split(wrapped, "\n")
		if len(segments) == 0 {
			segments = []string{""}
		}

		for index, segment := range segments {
			prefix := currentPrefix
			if index > 0 {
				prefix = nextPrefix
			}
			lines = append(lines, prefix+segment)
		}

		currentPrefix = nextPrefix
	}

	if len(lines) == 0 {
		return []string{firstPrefix}
	}
	return lines
}
