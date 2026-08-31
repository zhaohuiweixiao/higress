package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func requestWithToolCount(count int) string {
	tools := make([]string, 0, count)
	for i := 0; i < count; i++ {
		tools = append(tools, fmt.Sprintf(`{"type":"function","function":{"name":"f_%d"}}`, i))
	}
	return fmt.Sprintf(`{"model":"deepseek-v4-flash","thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hi"}],"tools":[%s]}`, strings.Join(tools, ","))
}

func TestDeepSeekRequestValidatorToolCallStateMachine(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		ruleID string
		param  string
	}{
		{
			name: "valid thinking tool call",
			body: `{
				"model":"deepseek-v4-flash",
				"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}],
				"messages":[
					{"role":"user","content":"weather"},
					{"role":"assistant","content":null,"reasoning_content":"need weather","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},
					{"role":"tool","tool_call_id":"call_1","content":"sunny"}
				]
			}`,
		},
		{
			name: "valid non-thinking tool call without reasoning content",
			body: `{
				"model":"deepseek-v4-flash",
				"thinking":{"type":"disabled"},
				"tools":[{"type":"function","function":{"name":"get_weather"}}],
				"messages":[
					{"role":"user","content":"weather"},
					{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},
					{"role":"tool","tool_call_id":"call_1","content":"sunny"}
				]
			}`,
		},
		{
			name: "valid thinking conversation without tools or reasoning content",
			body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hello"},{"role":"user","content":"again"}]}`,
		},
		{
			name:   "orphan tool message",
			body:   `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"weather"},{"role":"tool","tool_call_id":"call_1","content":"sunny"}]}`,
			ruleID: deepSeekRuleOrphanTool,
			param:  "messages[1].tool_call_id",
		},
		{
			name: "unknown tool call id",
			body: `{
				"model":"deepseek-v4-flash",
				"messages":[
					{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}}]},
					{"role":"tool","tool_call_id":"call_2","content":"ok"}
				]
			}`,
			ruleID: deepSeekRuleUnknownToolCall,
			param:  "messages[1].tool_call_id",
		},
		{
			name: "duplicate tool response",
			body: `{
				"model":"deepseek-v4-flash",
				"messages":[
					{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}}]},
					{"role":"tool","tool_call_id":"call_1","content":"ok"},
					{"role":"tool","tool_call_id":"call_1","content":"again"}
				]
			}`,
			ruleID: deepSeekRuleDuplicateToolResponse,
			param:  "messages[2].tool_call_id",
		},
		{
			name: "incomplete parallel tool results",
			body: `{
				"model":"deepseek-v4-flash",
				"messages":[
					{"role":"assistant","content":null,"tool_calls":[
						{"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}},
						{"id":"call_2","type":"function","function":{"name":"g","arguments":"{}"}}
					]},
					{"role":"tool","tool_call_id":"call_1","content":"ok"},
					{"role":"user","content":"continue"}
				]
			}`,
			ruleID: deepSeekRuleIncompleteToolResults,
			param:  "messages[2]",
		},
		{
			name: "missing reasoning content with tools",
			body: `{
				"model":"deepseek-v4-flash",
				"tools":[{"type":"function","function":{"name":"f"}}],
				"messages":[
					{"role":"user","content":"run"},
					{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}}]},
					{"role":"tool","tool_call_id":"call_1","content":"ok"}
				]
			}`,
			ruleID: deepSeekRuleMissingReasoningContent,
			param:  "messages[1].reasoning_content",
		},
		{
			name: "missing reasoning content on assistant without tool call",
			body: `{
				"model":"deepseek-v4-flash",
				"tools":[{"type":"function","function":{"name":"f"}}],
				"messages":[
					{"role":"user","content":"hello"},
					{"role":"assistant","content":"hello"},
					{"role":"user","content":"run"}
				]
			}`,
			ruleID: deepSeekRuleMissingReasoningContent,
			param:  "messages[1].reasoning_content",
		},
	}

	validator := deepSeekRequestValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate([]byte(tt.body), false)
			if tt.ruleID == "" {
				require.NoError(t, err)
				return
			}
			var validationErr *RequestValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, tt.ruleID, validationErr.RuleID)
			require.Equal(t, tt.param, validationErr.Param)
		})
	}
}

func TestDeepSeekRequestValidatorParameters(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		ruleID string
	}{
		{name: "invalid JSON", body: `{"messages":`, ruleID: deepSeekRuleInvalidJSON},
		{name: "empty messages", body: `{"model":"deepseek-v4-flash","messages":[]}`, ruleID: deepSeekRuleMessagesRequired},
		{name: "too many tools", body: requestWithToolCount(129), ruleID: deepSeekRuleTooManyTools},
		{name: "invalid function name", body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"bad name"}}],"thinking":{"type":"disabled"}}`, ruleID: deepSeekRuleInvalidFunctionName},
		{name: "stream options without stream", body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream_options":{"include_usage":true}}`, ruleID: deepSeekRuleInvalidStreamOptions},
		{name: "top logprobs requires logprobs", body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"top_logprobs":2}`, ruleID: deepSeekRuleInvalidTopLogprobs},
		{name: "invalid tool choice target", body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":{"type":"function","function":{"name":"g"}},"thinking":{"type":"disabled"}}`, ruleID: deepSeekRuleInvalidToolChoice},
		{name: "fractional top logprobs", body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"logprobs":true,"top_logprobs":1.5}`, ruleID: deepSeekRuleInvalidTopLogprobs},
		{name: "invalid stream option type", body: `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":"true"}}`, ruleID: deepSeekRuleInvalidStreamOptions},
	}

	validator := deepSeekRequestValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var validationErr *RequestValidationError
			require.ErrorAs(t, validator.Validate([]byte(tt.body), false), &validationErr)
			require.Equal(t, tt.ruleID, validationErr.RuleID)
		})
	}
}

func TestDeepSeekRequestValidatorStrictSchema(t *testing.T) {
	tests := []struct {
		name      string
		functions string
		wantError bool
	}{
		{
			name:      "valid strict schema",
			functions: `{"type":"function","function":{"name":"weather","strict":true,"parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}}}`,
		},
		{
			name:      "property not required",
			functions: `{"type":"function","function":{"name":"weather","strict":true,"parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":[],"additionalProperties":false}}}`,
			wantError: true,
		},
		{
			name:      "unsupported max length",
			functions: `{"type":"function","function":{"name":"weather","strict":true,"parameters":{"type":"object","properties":{"city":{"type":"string","maxLength":10}},"required":["city"],"additionalProperties":false}}}`,
			wantError: true,
		},
	}

	validator := deepSeekRequestValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"deepseek-v4-flash","thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hi"}],"tools":[%s]}`, tt.functions)
			err := validator.Validate([]byte(body), true)
			if !tt.wantError {
				require.NoError(t, err)
				return
			}
			var validationErr *RequestValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, deepSeekRuleInvalidStrictSchema, validationErr.RuleID)
		})
	}
}

func TestRequestValidationConfig(t *testing.T) {
	var config ProviderConfig
	config.FromJson(gjson.Parse(`{
		"type":"openai",
		"apiTokens":["token"],
		"requestValidation":{
			"enabled":true,
			"profile":"deepseek-chat-v4",
			"mode":"enforce",
			"modelPatterns":["deepseek-v4-*"],
			"validateStrictSchema":true
		}
	}`))

	require.NoError(t, config.Validate())
	require.True(t, config.requestValidation.enabled)
	require.Equal(t, requestValidationProfileDeepSeekV4, config.requestValidation.profile)
	require.True(t, config.requestValidation.validateStrictSchema)
	require.True(t, config.requestValidation.matchesModel("deepseek-v4-flash"))
	require.False(t, config.requestValidation.matchesModel("qwen-plus"))
}

func TestRequestValidationConfigRejectsUnknownValues(t *testing.T) {
	tests := []string{
		`{"type":"openai","apiTokens":["token"],"requestValidation":{"enabled":true,"profile":"unknown"}}`,
		`{"type":"openai","apiTokens":["token"],"requestValidation":{"enabled":true,"profile":"deepseek-chat-v4","mode":"invalid"}}`,
		`{"type":"openai","apiTokens":["token"],"requestValidation":{"enabled":true,"profile":"deepseek-chat-v4","modelPatterns":["["]}}`,
	}

	for _, body := range tests {
		var config ProviderConfig
		config.FromJson(gjson.Parse(body))
		require.Error(t, config.Validate())
	}
}

func TestRequestValidationErrorSupportsErrorsAs(t *testing.T) {
	err := newRequestValidationError("RULE", "messages[0]", "bad request")
	var target *RequestValidationError
	require.True(t, errors.As(err, &target))
	require.Equal(t, "bad request", target.Error())
}
