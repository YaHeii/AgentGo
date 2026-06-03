package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

func TestWebSearchToolMetadata(t *testing.T) {
	tool := NewWebSearchTool("https://api.anysearch.com/v1/search", "test-key")

	meta := tool.Metadata()
	require.Equal(t, WebSearchToolName, meta.Name)
	require.True(t, meta.Enabled)
	require.Equal(t, toolcontract.AttentionLevel, meta.SecurityLevel)
	require.Equal(t, toolcontract.RequireNetwork, meta.Requirements)
	require.True(t, meta.IsConcurrencySafe)
	require.Contains(t, meta.Description, "外部")
	require.JSONEq(t, `{
		"type": "object",
		"properties": {
			"query": { "type": "string", "description": "搜索查询文本，应直接描述要查找的主题、问题或事实。" },
			"max_results": { "type": "integer", "description": "期望返回的最大结果数。留空时使用服务端默认值；如果提供，必须在 1 到 100 之间。" },
			"domains": { "type": "array", "description": "可选的搜索域或数据源类别列表，用于限制检索范围，例如特定站点组或垂直域。", "items": { "type": "string" } },
			"tags": { "type": "array", "description": "可选的标签过滤条件，用于进一步缩小结果范围。", "items": { "type": "string" } },
			"content_types": { "type": "array", "description": "可选的内容类型过滤条件，例如网页、文档或其他服务端支持的类型。", "items": { "type": "string" } },
			"zone": { "type": "string", "description": "可选的区域或市场范围，用于偏向某个地理搜索区域。" },
			"language": { "type": "string", "description": "可选的语言过滤条件，用于偏向某种结果语言。" },
			"providers": { "type": "array", "description": "可选的底层搜索提供方列表，用于限制 AnySearch 使用的 provider。", "items": { "type": "string" } },
			"freshness": { "type": "string", "description": "可选的时间新鲜度约束，例如 day、week、month 或 year；会作为 AnySearch constraint 发送。" },
			"from": { "type": "string", "description": "可选的起始日期，通常使用 YYYY-MM-DD 或服务端可识别的日期格式，与 to 共同限定时间范围。" },
			"to": { "type": "string", "description": "可选的结束日期，通常使用 YYYY-MM-DD 或服务端可识别的日期格式，与 from 共同限定时间范围。" }
		},
		"required": ["query"]
	}`, string(meta.Parameters))
}

func TestWebSearchToolExecuteCallsAnySearch(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	tool := NewWebSearchTool("https://api.anysearch.com/v1/search", "test-key")
	tool.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "https://api.anysearch.com/v1/search", r.URL.String())
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewBufferString(`{
			"results": [
				{
					"title": "Go 1.22 Release Notes",
					"url": "https://go.dev/doc/go1.22",
					"description": "Go 1.22 is a major release.",
					"content": "Detailed content here.",
					"source": "doc",
					"score": 0.87,
					"quality_score": 0.95,
					"published_at": "2024-02-06T00:00:00Z"
				}
			],
			"metadata": {
				"total_results": 1,
				"search_time_ms": 342,
				"routes_queried": 2,
				"routes_succeeded": 2,
				"request_id": "req_abc123",
				"cached": false
			}
		}`)),
		}, nil
	})}
	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            WebSearchToolName,
		PermissionLevel: toolcontract.AttentionLevel,
		Arguments: json.RawMessage(`{
			"query": "Go 1.22 release notes",
			"max_results": 5,
			"domains": ["code", "tech"],
			"content_types": ["web", "doc"],
			"freshness": "year"
		}`),
	})

	require.Equal(t, "Bearer test-key", gotAuth)
	require.Equal(t, "Go 1.22 release notes", gotBody["query"])
	require.Equal(t, float64(5), gotBody["max_results"])
	require.Equal(t, []any{"code", "tech"}, gotBody["domains"])
	require.Equal(t, []any{"web", "doc"}, gotBody["content_types"])
	require.Equal(t, map[string]any{"freshness": "year"}, gotBody["constraint"])
	require.Equal(t, toolcontract.StatusSuccess, result.Status)
	require.NoError(t, result.Err)
	require.Contains(t, result.Content, "Go 1.22 Release Notes")
	require.Contains(t, result.Content, "https://go.dev/doc/go1.22")
	require.Contains(t, result.Content, "Detailed content here.")
	require.Equal(t, "req_abc123", result.Metadata["request_id"])
	require.Equal(t, 1, result.Metadata["result_count"])
}

func TestWebSearchToolExecuteValidatesQuery(t *testing.T) {
	tool := NewWebSearchTool("https://api.anysearch.com/v1/search", "test-key")

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            WebSearchToolName,
		PermissionLevel: toolcontract.AttentionLevel,
		Arguments:       json.RawMessage(`{"query":"   "}`),
	})

	require.Equal(t, toolcontract.StatusValidationFailed, result.Status)
	require.Error(t, result.Err)
	require.Contains(t, result.Content, "query is required")
}

func TestWebSearchToolExecuteMapsAnySearchError(t *testing.T) {
	tool := NewWebSearchTool("https://api.anysearch.com/v1/search", "bad-key")
	tool.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewBufferString(`{
			"symbol": "invalid_api_key",
			"message": "API key is invalid",
			"request_id": "req_bad_key"
		}`)),
		}, nil
	})}
	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            WebSearchToolName,
		PermissionLevel: toolcontract.AttentionLevel,
		Arguments:       json.RawMessage(`{"query":"Go"}`),
	})

	require.Equal(t, toolcontract.StatusSystemError, result.Status)
	require.Error(t, result.Err)
	require.Contains(t, result.Content, "invalid_api_key")
	require.Equal(t, "req_bad_key", result.Metadata["request_id"])
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
