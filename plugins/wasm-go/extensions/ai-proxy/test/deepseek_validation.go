package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func deepSeekValidationConfig(providerType, mode string) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      providerType,
			"apiTokens": []string{"test-token"},
			"requestValidation": map[string]interface{}{
				"enabled":       true,
				"profile":       "deepseek-chat-v4",
				"mode":          mode,
				"modelPatterns": []string{"deepseek-v4-*"},
			},
		},
	})
	return data
}

func RunDeepSeekRequestValidationTests(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		t.Run("enforce returns DeepSeek-compatible local 400", func(t *testing.T) {
			for _, providerType := range []string{"deepseek", "openai"} {
				t.Run(providerType, func(t *testing.T) {
					host, status := wasmtest.NewTestHost(deepSeekValidationConfig(providerType, "enforce"))
					defer host.Reset()
					require.Equal(t, types.OnPluginStartStatusOK, status)

					host.CallOnHttpRequestHeaders([][2]string{
						{":authority", "example.com"},
						{":path", "/v1/chat/completions"},
						{":method", "POST"},
						{"Content-Type", "application/json"},
					})
					host.CallOnHttpRequestBody([]byte(`{
				"model":"deepseek-v4-flash",
				"messages":[
					{"role":"user","content":"weather"},
					{"role":"tool","tool_call_id":"call_1","content":"sunny"}
				]
			}`))

					response := host.GetLocalResponse()
					require.NotNil(t, response)
					require.Equal(t, uint32(400), response.StatusCode)
					require.Equal(t, "ai-proxy.request_validation.ds_tool_001", response.StatusCodeDetail)
					require.Equal(t, "invalid_request_error", gjson.GetBytes(response.Data, "error.type").String())
					require.Equal(t, "invalid_request_error", gjson.GetBytes(response.Data, "error.code").String())
					require.Equal(t, "messages[1].tool_call_id", gjson.GetBytes(response.Data, "error.param").String())
				})
			}
		})

		t.Run("shadow observes but forwards invalid request", func(t *testing.T) {
			host, status := wasmtest.NewTestHost(deepSeekValidationConfig("openai", "shadow"))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{
				"model":"deepseek-v4-flash",
				"messages":[{"role":"tool","tool_call_id":"call_1","content":"sunny"}]
			}`))

			require.Nil(t, host.GetLocalResponse())
			require.NotNil(t, host.GetRequestBody())
		})
	})
}
