package contract

import (
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestReferencesSharedMessageAndToolContracts(t *testing.T) {
	temperature := float32(0.2)
	maxOutputTokens := 256

	req := Request{
		Messages: []messagecontract.Message{
			{
				ID:   "msg-1",
				Kind: messagecontract.KindUser,
				Parts: []messagecontract.Part{
					{Type: messagecontract.PartTypeText, Text: "hello"},
				},
			},
		},
		Tools: []toolcontract.Metadata{
			{Name: "grep", Description: "search"},
		},
		Context: RequestContext{
			Temperature:     &temperature,
			MaxOutputTokens: &maxOutputTokens,
		},
	}

	require.Len(t, req.Messages, 1)
	require.Equal(t, messagecontract.KindUser, req.Messages[0].Kind)
	require.Len(t, req.Tools, 1)
	require.Equal(t, "grep", req.Tools[0].Name)
	require.Equal(t, float32(0.2), *req.Context.Temperature)
	require.Equal(t, 256, *req.Context.MaxOutputTokens)
}
