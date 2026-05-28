package rag

import (
	"fmt"
	"strings"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

func assembleContextMessage(matches []ragcontract.ChunkMatch) messagecontract.Message {
	return messagecontract.Message{
		Kind: messagecontract.KindSystem,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
				Text: buildContextText(matches),
			},
		},
		System: &messagecontract.SystemPayload{
			Subtype: "rag_context",
			Level:   "info",
		},
	}
}

func buildContextText(matches []ragcontract.ChunkMatch) string {
	if len(matches) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("以下内容是检索得到的参考上下文，仅在与当前问题相关时使用。\n\n")
	for i, match := range matches {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Chunk %d\n", i+1)
		fmt.Fprintf(&b, "Source: %s\n", match.Document.SourcePath)
		b.WriteString("Content:\n")
		b.WriteString(strings.TrimSpace(match.Chunk.Content))
	}
	return b.String()
}
