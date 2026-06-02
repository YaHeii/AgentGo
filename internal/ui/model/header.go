package model

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/YaHeii/agentGo/internal/lifecycle"
)

const headerHeight = 6

type Header struct {
	transientStatus string
}

func newHeader() Header {
	return Header{transientStatus: defaultHeaderStatus}
}

func (h Header) Height() int {
	return headerHeight
}

func (h *Header) SetTransientStatus(status string) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = defaultHeaderStatus
	}
	h.transientStatus = status
}

func (h Header) TransientStatus() string {
	return h.transientStatus
}

func (h Header) View(width int) string {
	lines := []string{
		"agent: " + headerSessionID(),
		ellipsizeLeft("cwd: "+headerCWD(), width),
		joinHeaderLine("model", headerModel(), "permission", headerPermission()),
		joinHeaderLine(
			"in/out/total",
			headerCurrentTokens(),
			"context",
			headerContextTokens(),
		),
		joinHeaderLine(
			"cum in/out/total",
			headerCumulativeTokens(),
			"messages",
			headerMessageCount(),
		),
		h.transientStatus,
	}

	for index := range lines {
		lines[index] = truncateLine(lines[index], width)
	}
	return strings.Join(lines, "\n")
}

func headerSessionID() string {
	if lifecycle.State == nil {
		return ""
	}
	return strings.TrimSpace(lifecycle.State.SessionID)
}

func headerCWD() string {
	if lifecycle.State == nil {
		return ""
	}
	cwd := strings.TrimSpace(lifecycle.State.Cwd)
	root := strings.TrimSpace(lifecycle.State.ProjectRoot)
	if cwd == "" {
		return ""
	}
	if root != "" {
		if relative, err := filepath.Rel(root, cwd); err == nil && relative != "." && relative != "" {
			return filepath.ToSlash(relative)
		}
		if cwd == root {
			return "."
		}
	}
	return filepath.ToSlash(cwd)
}

func headerModel() string {
	if lifecycle.State == nil {
		return ""
	}
	return strings.TrimSpace(lifecycle.State.Model)
}

func headerPermission() string {
	if lifecycle.State == nil {
		return ""
	}
	return permissionLabel(lifecycle.State.PermissionLevel)
}

func permissionLabel(level lifecycle.PermissionLevel) string {
	switch level {
	case lifecycle.DangerLevel:
		return "danger"
	case lifecycle.AttentionLevel:
		return "attention"
	default:
		return "safe"
	}
}

func headerCurrentTokens() string {
	if lifecycle.State == nil {
		return ""
	}
	return fmt.Sprintf(
		"%d/%d/%d",
		lifecycle.State.CurrentTurnInputTokens,
		lifecycle.State.CurrentTurnOutputTokens,
		lifecycle.State.CurrentTurnTotalTokens,
	)
}

func headerContextTokens() string {
	if lifecycle.State == nil {
		return ""
	}
	return fmt.Sprintf("%d", lifecycle.State.ActualContextTokens)
}

func headerCumulativeTokens() string {
	if lifecycle.State == nil {
		return ""
	}
	return fmt.Sprintf(
		"%d/%d/%d",
		lifecycle.State.CumulativeInputTokens,
		lifecycle.State.CumulativeOutputTokens,
		lifecycle.State.CumulativeTotalTokens,
	)
}

func headerMessageCount() string {
	if lifecycle.State == nil {
		return ""
	}
	return fmt.Sprintf("%d", lifecycle.State.CurrentMessageCount)
}

func joinHeaderLine(leftLabel string, leftValue string, rightLabel string, rightValue string) string {
	return leftLabel + ": " + leftValue + "  " + rightLabel + ": " + rightValue
}

func truncateLine(text string, width int) string {
	if width <= 0 {
		return text
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(text)
}

func ellipsizeLeft(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width("..."+string(runes)) > width {
		runes = runes[1:]
	}
	return "..." + string(runes)
}
