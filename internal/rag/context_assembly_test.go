package rag

import (
	"testing"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
	"github.com/stretchr/testify/require"
)

func TestAssembleContextMessageReturnsSystemMessage(t *testing.T) {
	t.Parallel()

	msg := assembleContextMessage([]ragcontract.ChunkMatch{
		{
			Chunk: ragcontract.Chunk{
				ChunkIndex: 0,
				Content:    "alpha content",
			},
			Document: ragcontract.Document{
				SourcePath: "docs/alpha.md",
			},
		},
		{
			Chunk: ragcontract.Chunk{
				ChunkIndex: 1,
				Content:    "beta content",
			},
			Document: ragcontract.Document{
				SourcePath: "docs/beta.md",
			},
		},
	})

	require.Equal(t, messagecontract.KindSystem, msg.Kind)
	require.NotNil(t, msg.System)
	require.Equal(t, "rag_context", msg.System.Subtype)
	require.Equal(t, "info", msg.System.Level)
	require.Len(t, msg.Parts, 1)
	require.Equal(t, messagecontract.PartTypeText, msg.Parts[0].Type)
	require.Contains(t, msg.Parts[0].Text, "以下内容是检索得到的参考上下文")
	require.Contains(t, msg.Parts[0].Text, "Chunk 1")
	require.Contains(t, msg.Parts[0].Text, "Source: docs/alpha.md")
	require.Contains(t, msg.Parts[0].Text, "Content:\nalpha content")
	require.Contains(t, msg.Parts[0].Text, "Chunk 2")
	require.Contains(t, msg.Parts[0].Text, "Source: docs/beta.md")
}

func TestAssembleContextMessageReturnsEmptyTextWhenNoMatches(t *testing.T) {
	t.Parallel()

	msg := assembleContextMessage(nil)

	require.Equal(t, messagecontract.KindSystem, msg.Kind)
	require.NotNil(t, msg.System)
	require.Len(t, msg.Parts, 1)
	require.Equal(t, "", msg.Parts[0].Text)
}
