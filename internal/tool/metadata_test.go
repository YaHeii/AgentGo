package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type metadataSchemaNode struct {
	Type        string                        `json:"type"`
	Description string                        `json:"description"`
	Properties  map[string]metadataSchemaNode `json:"properties"`
	Required    []string                      `json:"required"`
	Items       *metadataSchemaNode           `json:"items"`
}

func TestLocalToolMetadataProvidesActionableDescriptions(t *testing.T) {
	t.Parallel()

	tools := []Tool{
		NewBashToolWithRunner(nil),
		NewEditTool(),
		NewGrepTool(t.TempDir()),
		NewTodosTool(nil),
		NewWebSearchTool("https://example.com/search", "key"),
	}

	for _, tool := range tools {
		meta := tool.Metadata()
		require.NotEmpty(t, strings.TrimSpace(meta.Name), "tool name must not be empty")
		require.NotEmpty(t, strings.TrimSpace(meta.Description), "tool %s description must not be empty", meta.Name)

		var schema metadataSchemaNode
		require.NoError(t, json.Unmarshal(meta.Parameters, &schema), "tool %s parameters must be valid JSON schema", meta.Name)
		require.Equal(t, "object", schema.Type, "tool %s parameters schema must be an object", meta.Name)
		require.NotEmpty(t, schema.Properties, "tool %s parameters schema must define properties", meta.Name)

		assertSchemaPropertyDescriptions(t, meta.Name, "arguments", schema)
	}
}

func assertSchemaPropertyDescriptions(t *testing.T, toolName string, path string, schema metadataSchemaNode) {
	t.Helper()

	for propertyName, property := range schema.Properties {
		propertyPath := path + "." + propertyName
		require.NotEmpty(
			t,
			strings.TrimSpace(property.Description),
			"tool %s property %s must define description",
			toolName,
			propertyPath,
		)

		if property.Type == "object" && len(property.Properties) > 0 {
			assertSchemaPropertyDescriptions(t, toolName, propertyPath, property)
		}
		if property.Type == "array" && property.Items != nil && property.Items.Type == "object" {
			assertSchemaPropertyDescriptions(t, toolName, propertyPath+"[]", *property.Items)
		}
	}
}
