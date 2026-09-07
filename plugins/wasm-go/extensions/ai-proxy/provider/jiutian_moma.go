package provider

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	jiutianMomaDomain = "moma.hq.cmcc"
	// http://moma.hq.cmcc/largemodel/llm-api-help-center/#/document/chat-api
	jiutianMomaChatCompletionPath = "/api/v3/chat/completions"
	// http://moma.hq.cmcc/largemodel/llm-api-help-center/#/document/anthropic-chat-api
	jiutianMomaMessagesPath = "/api/v1/messages"
)

type jiutianMomaProviderInitializer struct{}

func (m *jiutianMomaProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *jiutianMomaProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion):    jiutianMomaChatCompletionPath,
		string(ApiNameAnthropicMessages): jiutianMomaMessagesPath,
	}
}

func (m *jiutianMomaProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &jiutianMomaProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type jiutianMomaProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (m *jiutianMomaProvider) GetProviderType() string {
	return providerTypeJiutianMoma
}

func (m *jiutianMomaProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *jiutianMomaProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func (m *jiutianMomaProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	if m.config.jiutianMomaDomain != "" {
		util.OverwriteRequestHostHeader(headers, m.config.jiutianMomaDomain)
	} else {
		util.OverwriteRequestHostHeader(headers, jiutianMomaDomain)
	}
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (m *jiutianMomaProvider) TransformResponseHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	headers.Del("Content-Length")
}

func (m *jiutianMomaProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName == ApiNameChatCompletion && m.config.responseJsonSchema != nil {
		request := &chatCompletionRequest{}
		if err := decodeChatCompletionRequest(body, request); err != nil {
			return nil, err
		}
		request.ResponseFormat = m.config.responseJsonSchema
		updatedBody, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		body = updatedBody
	}
	if apiName == ApiNameChatCompletion {
		var err error
		body, err = transformJiutianMomaReasoning(body)
		if err != nil {
			return nil, err
		}
	}
	if apiName == ApiNameAnthropicMessages {
		var err error
		body, err = transformJiutianMomaAnthropicOutputConfig(body)
		if err != nil {
			return nil, err
		}
	}
	return m.config.defaultTransformRequestBody(ctx, apiName, body)
}

func (m *jiutianMomaProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameAnthropicMessages || len(body) == 0 {
		return body, nil
	}
	return ensureAnthropicUsageFields(body)
}

func (m *jiutianMomaProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, apiName ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if apiName != ApiNameAnthropicMessages || len(chunk) == 0 {
		return chunk, nil
	}

	lines := strings.Split(string(chunk), "\n")
	var builder strings.Builder
	for i, line := range lines {
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload != "" && payload != "[DONE]" {
				modifiedPayload, err := ensureAnthropicUsageFields([]byte(payload))
				if err != nil {
					return nil, err
				}
				line = "data: " + string(modifiedPayload)
			}
		}
		builder.WriteString(line)
		if i < len(lines)-1 {
			builder.WriteByte('\n')
		}
	}
	return []byte(builder.String()), nil
}

func (m *jiutianMomaProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, jiutianMomaChatCompletionPath) || strings.Contains(path, PathOpenAIChatCompletions) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, jiutianMomaMessagesPath) || strings.Contains(path, PathAnthropicMessages) {
		return ApiNameAnthropicMessages
	}
	return ""
}

func ensureAnthropicUsageFields(body []byte) ([]byte, error) {
	modifiedBody := body
	for _, usagePath := range []string{"usage", "message.usage"} {
		usage := gjson.GetBytes(modifiedBody, usagePath)
		if !usage.Exists() || usage.Type == gjson.Null {
			continue
		}
		inputTokens := gjson.GetBytes(modifiedBody, usagePath+".input_tokens")
		if !inputTokens.Exists() || inputTokens.Type == gjson.Null {
			var err error
			modifiedBody, err = sjson.SetBytes(modifiedBody, usagePath+".input_tokens", 0)
			if err != nil {
				return nil, err
			}
		}

		totalTokens := gjson.GetBytes(modifiedBody, usagePath+".total_tokens")
		if !totalTokens.Exists() || totalTokens.Type == gjson.Null {
			var err error
			modifiedBody, err = sjson.SetBytes(modifiedBody, usagePath+".total_tokens", calculateAnthropicTotalTokens(modifiedBody, usagePath))
			if err != nil {
				return nil, err
			}
		}
	}
	return modifiedBody, nil
}

func calculateAnthropicTotalTokens(body []byte, usagePath string) int64 {
	inputTokens := gjson.GetBytes(body, usagePath+".input_tokens").Int()
	outputTokens := gjson.GetBytes(body, usagePath+".output_tokens").Int()
	cacheReadInputTokens := gjson.GetBytes(body, usagePath+".cache_read_input_tokens").Int()
	cacheCreationInputTokens := getAnthropicCacheCreationInputTokens(body, usagePath)
	return inputTokens + outputTokens + cacheReadInputTokens + cacheCreationInputTokens
}

func getAnthropicCacheCreationInputTokens(body []byte, usagePath string) int64 {
	cacheCreationInputTokens := gjson.GetBytes(body, usagePath+".cache_creation_input_tokens")
	if cacheCreationInputTokens.Exists() && cacheCreationInputTokens.Type != gjson.Null {
		return cacheCreationInputTokens.Int()
	}

	var total int64
	for _, path := range []string{
		usagePath + ".cache_creation.ephemeral_5m_input_tokens",
		usagePath + ".cache_creation.ephemeral_1h_input_tokens",
	} {
		value := gjson.GetBytes(body, path)
		if value.Exists() && value.Type != gjson.Null {
			total += value.Int()
		}
	}
	return total
}

func transformJiutianMomaReasoning(body []byte) ([]byte, error) {
	reasoningEffort := gjson.GetBytes(body, "reasoning_effort")
	if !reasoningEffort.Exists() || reasoningEffort.Type == gjson.Null {
		return body, nil
	}

	effort := strings.ToLower(strings.TrimSpace(reasoningEffort.String()))
	if effort == "" {
		return body, nil
	}
	if !isSupportedJiutianMomaReasoningEffort(effort) {
		return nil, &InvalidParameterError{
			Param:   "reasoning_effort",
			Message: "reasoning_effort must be one of: none, minimal, low, medium, high, xhigh",
		}
	}

	modifiedBody, err := sjson.SetBytes(body, "reasoning.enabled", effort != "none")
	if err != nil {
		return nil, err
	}
	modifiedBody, err = sjson.SetBytes(modifiedBody, "reasoning.effort", effort)
	if err != nil {
		return nil, err
	}
	modifiedBody, err = sjson.DeleteBytes(modifiedBody, "reasoning_effort")
	if err != nil {
		return nil, err
	}
	return modifiedBody, nil
}

func isSupportedJiutianMomaReasoningEffort(effort string) bool {
	switch effort {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func transformJiutianMomaAnthropicOutputConfig(body []byte) ([]byte, error) {
	outputConfigEffort := gjson.GetBytes(body, "output_config.effort")
	if !outputConfigEffort.Exists() || outputConfigEffort.Type == gjson.Null {
		return body, nil
	}

	effort := strings.ToLower(strings.TrimSpace(outputConfigEffort.String()))
	if effort == "" {
		return body, nil
	}

	mappedEffort, ok := mapAnthropicOutputConfigEffortToJiutianMoma(effort)
	if !ok {
		return nil, &InvalidParameterError{
			Param:   "output_config.effort",
			Message: "output_config.effort must be one of: low, medium, high, xhigh, max",
		}
	}

	return sjson.SetBytes(body, "output_config.effort", mappedEffort)
}

func mapAnthropicOutputConfigEffortToJiutianMoma(effort string) (string, bool) {
	switch effort {
	case "minimal":
		return "minimal", true
	case "low":
		return "minimal", true
	case "medium":
		return "low", true
	case "high":
		return "medium", true
	case "xhigh":
		return "high", true
	case "max":
		return "xhigh", true
	default:
		return "", false
	}
}
