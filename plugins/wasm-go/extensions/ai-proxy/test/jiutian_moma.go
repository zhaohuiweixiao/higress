package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicJiutianMomaConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "jiutian_moma",
			"apiTokens": []string{"jiutian-moma-token"},
			"modelMapping": map[string]string{
				"*": "jiutian/jiutian-lan-35b",
			},
		},
	})
	return data
}()

var customJiutianMomaDomainConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":              "jiutian_moma",
			"apiTokens":         []string{"jiutian-moma-token"},
			"jiutianMomaDomain": "moma.internal.example",
		},
	})
	return data
}()

var invalidJiutianMomaConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "jiutian_moma",
		},
	})
	return data
}()

func RunJiutianMomaParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic jiutian moma config", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("custom jiutian moma domain config", func(t *testing.T) {
			host, status := test.NewTestHost(customJiutianMomaDomainConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("jiutian moma config without token fails", func(t *testing.T) {
			host, status := test.NewTestHost(invalidJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func RunJiutianMomaOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("chat completions headers route to moma v3 endpoint", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "moma.hq.cmcc"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, "Authorization", "Bearer jiutian-moma-token"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":path", "/largemodel/moma/api/v3/chat/completions"))
		})

		t.Run("messages headers keep native anthropic route", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "moma.hq.cmcc"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":path", "/largemodel/moma/api/v1/messages"))
		})

		t.Run("custom domain overrides default host", func(t *testing.T) {
			host, status := test.NewTestHost(customJiutianMomaDomainConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "moma.internal.example"))
		})
	})
}

func RunJiutianMomaOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("chat completions body keeps openai shape and applies model mapping", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
				"model":"external-model",
				"messages":[{"role":"user","content":"hello"}],
				"stream":false
			}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			var request map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &request)
			require.NoError(t, err)
			require.Equal(t, "jiutian/jiutian-lan-35b", request["model"])
			require.Contains(t, request, "messages")
		})

		t.Run("chat completions body maps reasoning_effort to vendor reasoning", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
                                "model":"external-model",
                                "messages":[{"role":"user","content":"hello"}],
                                "reasoning_effort":"none",
                                "stream":false
                        }`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			var request map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &request)
			require.NoError(t, err)
			require.Equal(t, "jiutian/jiutian-lan-35b", request["model"])

			reasoning, ok := request["reasoning"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, false, reasoning["enabled"])
			require.Equal(t, "none", reasoning["effort"])
			_, exists := request["reasoning_effort"]
			require.False(t, exists)
		})

		t.Run("chat completions reasoning_effort overrides vendor reasoning", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
                                "model":"external-model",
                                "messages":[{"role":"user","content":"hello"}],
                                "reasoning_effort":"high",
                                "reasoning":{"enabled":false,"effort":"none"},
                                "stream":false
                        }`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			var request map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &request)
			require.NoError(t, err)

			reasoning, ok := request["reasoning"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, true, reasoning["enabled"])
			require.Equal(t, "high", reasoning["effort"])
			_, exists := request["reasoning_effort"]
			require.False(t, exists)
		})

		t.Run("messages body keeps anthropic shape and applies model mapping", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
				"model":"external-model",
				"max_tokens":256,
				"messages":[{"role":"user","content":"hello"}],
				"stream":false
			}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			var request map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &request)
			require.NoError(t, err)
			require.Equal(t, "jiutian/jiutian-lan-35b", request["model"])
			require.Contains(t, request, "max_tokens")
			require.Contains(t, request, "messages")
		})

		t.Run("messages body maps output_config effort by strength and preserves format", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
                                "model":"external-model",
                                "max_tokens":256,
                                "messages":[{"role":"user","content":"hello"}],
                                "output_config":{
                                        "effort":"high",
                                        "format":{"type":"json_object"}
                                },
                                "stream":false
                        }`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			var request map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &request)
			require.NoError(t, err)
			require.Equal(t, "jiutian/jiutian-lan-35b", request["model"])

			outputConfig, ok := request["output_config"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "medium", outputConfig["effort"])
			require.Equal(t, map[string]interface{}{"type": "json_object"}, outputConfig["format"])
		})
	})
}

func RunJiutianMomaOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("messages response body adds explicit zero input tokens", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			host.CallOnHttpRequestBody([]byte(`{
				"model":"external-model",
				"max_tokens":256,
				"messages":[{"role":"user","content":"hello"}],
				"stream":false
			}`))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})

			responseBody := `{
				"id":"msg-1",
				"type":"message",
				"usage":{
					"output_tokens":95,
					"cache_read_input_tokens":8,
					"total_tokens":103
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)
			require.JSONEq(t, `{
				"id":"msg-1",
				"type":"message",
				"usage":{
					"input_tokens":0,
					"output_tokens":95,
					"cache_read_input_tokens":8,
					"total_tokens":103
				}
			}`, string(host.GetResponseBody()))
		})

		t.Run("messages response body adds total tokens when missing", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			host.CallOnHttpRequestBody([]byte(`{
				"model":"external-model",
				"max_tokens":256,
				"messages":[{"role":"user","content":"hello"}],
				"stream":false
			}`))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})

			responseBody := `{
				"id":"msg-1",
				"type":"message",
				"usage":{
                                        "input_tokens":6,
                                        "output_tokens":32,
                                        "cache_read_input_tokens":8,
                                        "cache_creation_input_tokens":4
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)
			require.JSONEq(t, `{
				"id":"msg-1",
				"type":"message",
				"usage":{
					"input_tokens":6,
					"output_tokens":32,
                                        "cache_read_input_tokens":8,
                                        "cache_creation_input_tokens":4,
                                        "total_tokens":50
				}
			}`, string(host.GetResponseBody()))
		})

		t.Run("messages response body adds total tokens from cache creation breakdown when summary field is missing", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			host.CallOnHttpRequestBody([]byte(`{
                                "model":"external-model",
                                "max_tokens":256,
                                "messages":[{"role":"user","content":"hello"}],
                                "stream":false
                        }`))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})

			responseBody := `{
                                "id":"msg-1",
                                "type":"message",
                                "usage":{
                                        "input_tokens":6,
                                        "output_tokens":32,
                                        "cache_read_input_tokens":8,
                                        "cache_creation":{
                                                "ephemeral_5m_input_tokens":4
                                        }
                                }
                        }`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)
			require.JSONEq(t, `{
                                "id":"msg-1",
                                "type":"message",
                                "usage":{
                                        "input_tokens":6,
                                        "output_tokens":32,
                                        "cache_read_input_tokens":8,
                                        "cache_creation":{
                                                "ephemeral_5m_input_tokens":4
                                        },
                                        "total_tokens":50
                                }
                        }`, string(host.GetResponseBody()))
		})
	})
}

func RunJiutianMomaOnHttpStreamingResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("messages streaming response adds explicit zero input tokens", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			host.CallOnHttpRequestBody([]byte(`{
				"model":"external-model",
				"max_tokens":256,
				"messages":[{"role":"user","content":"hello"}],
				"stream":true
			}`))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			})

			chunk1 := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg-1","usage":{"output_tokens":0}}}` + "\n\n"
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)
			require.Contains(t, string(host.GetResponseBody()), `"usage":{"output_tokens":0,"input_tokens":0,"total_tokens":0}`)

			chunk2 := "event: message_delta\ndata: " + `{"type":"message_delta","delta":{"type":"","stop_reason":"end_turn"},"usage":{"output_tokens":95,"cache_read_input_tokens":8,"total_tokens":103}}` + "\n\n"
			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), true)
			require.Equal(t, types.ActionContinue, action2)
			require.Contains(t, string(host.GetResponseBody()), `"usage":{"output_tokens":95,"cache_read_input_tokens":8,"total_tokens":103,"input_tokens":0}`)
		})

		t.Run("messages streaming response adds total tokens when missing", func(t *testing.T) {
			host, status := test.NewTestHost(basicJiutianMomaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			host.CallOnHttpRequestBody([]byte(`{
				"model":"external-model",
				"max_tokens":256,
				"messages":[{"role":"user","content":"hello"}],
				"stream":true
			}`))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			})

			chunk1 := "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg-1","usage":{"output_tokens":0}}}` + "\n\n"
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)
			require.Contains(t, string(host.GetResponseBody()), `"usage":{"output_tokens":0,"input_tokens":0,"total_tokens":0}`)

			chunk2 := "event: message_delta\ndata: " + `{"type":"message_delta","delta":{"type":"","stop_reason":"end_turn"},"usage":{"input_tokens":13,"output_tokens":32,"cache_read_input_tokens":8,"cache_creation_input_tokens":4}}` + "\n\n"
			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), true)
			require.Equal(t, types.ActionContinue, action2)
			require.Contains(t, string(host.GetResponseBody()), `"usage":{"input_tokens":13,"output_tokens":32,"cache_read_input_tokens":8,"cache_creation_input_tokens":4,"total_tokens":57}`)
		})
	})
}
