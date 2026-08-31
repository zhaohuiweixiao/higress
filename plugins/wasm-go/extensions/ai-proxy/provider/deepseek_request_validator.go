package provider

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

const (
	requestValidationProfileDeepSeekV4 = "deepseek-chat-v4"
	requestValidationModeEnforce       = "enforce"
	requestValidationModeShadow        = "shadow"

	deepSeekRuleInvalidJSON             = "DS_REQ_001"
	deepSeekRuleModelRequired           = "DS_REQ_002"
	deepSeekRuleMessagesRequired        = "DS_MSG_001"
	deepSeekRuleInvalidRole             = "DS_MSG_002"
	deepSeekRuleInvalidMessage          = "DS_MSG_003"
	deepSeekRuleOrphanTool              = "DS_TOOL_001"
	deepSeekRuleUnknownToolCall         = "DS_TOOL_002"
	deepSeekRuleDuplicateToolResponse   = "DS_TOOL_003"
	deepSeekRuleIncompleteToolResults   = "DS_TOOL_004"
	deepSeekRuleDuplicateToolCall       = "DS_TOOL_005"
	deepSeekRuleInvalidToolCall         = "DS_TOOL_006"
	deepSeekRuleMissingReasoningContent = "DS_THINK_001"
	deepSeekRuleInvalidReasoningContent = "DS_THINK_002"
	deepSeekRuleInvalidParameter        = "DS_PARAM_001"
	deepSeekRuleInvalidStreamOptions    = "DS_PARAM_002"
	deepSeekRuleInvalidTopLogprobs      = "DS_PARAM_003"
	deepSeekRuleTooManyTools            = "DS_TOOLDEF_001"
	deepSeekRuleInvalidFunctionName     = "DS_TOOLDEF_002"
	deepSeekRuleInvalidToolDefinition   = "DS_TOOLDEF_003"
	deepSeekRuleInvalidToolChoice       = "DS_TOOLDEF_004"
	deepSeekRuleInvalidStrictSchema     = "DS_STRICT_001"
)

var (
	deepSeekFunctionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	deepSeekUserIDPattern       = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,512}$`)
)

// RequestValidationError is returned when an upstream request violates a
// provider-specific request contract. It intentionally contains no request
// content so callers can log it without leaking prompts or reasoning text.
type RequestValidationError struct {
	RuleID  string
	Param   string
	Message string
}

func (e *RequestValidationError) Error() string {
	return e.Message
}

func newRequestValidationError(ruleID, param, message string) error {
	return &RequestValidationError{RuleID: ruleID, Param: param, Message: message}
}

type requestValidationConfig struct {
	enabled              bool
	profile              string
	mode                 string
	modelPatterns        []string
	validateStrictSchema bool
}

func (c *requestValidationConfig) FromJSON(value gjson.Result) {
	c.enabled = value.Get("enabled").Bool()
	c.profile = value.Get("profile").String()
	c.mode = value.Get("mode").String()
	if c.mode == "" {
		c.mode = requestValidationModeEnforce
	}
	c.modelPatterns = c.modelPatterns[:0]
	for _, item := range value.Get("modelPatterns").Array() {
		if item.Type == gjson.String && item.String() != "" {
			c.modelPatterns = append(c.modelPatterns, item.String())
		}
	}
	c.validateStrictSchema = value.Get("validateStrictSchema").Bool()
}

func (c requestValidationConfig) Validate() error {
	if !c.enabled {
		return nil
	}
	if c.profile != requestValidationProfileDeepSeekV4 {
		return fmt.Errorf("unsupported request validation profile: %s", c.profile)
	}
	if c.mode != requestValidationModeEnforce && c.mode != requestValidationModeShadow {
		return fmt.Errorf("unsupported request validation mode: %s", c.mode)
	}
	for _, pattern := range c.modelPatterns {
		if _, err := path.Match(pattern, "model"); err != nil {
			return fmt.Errorf("invalid request validation model pattern %q: %v", pattern, err)
		}
	}
	return nil
}

func (c requestValidationConfig) matchesModel(model string) bool {
	if len(c.modelPatterns) == 0 {
		return true
	}
	for _, pattern := range c.modelPatterns {
		matched, _ := path.Match(pattern, model)
		if matched {
			return true
		}
	}
	return false
}

func (c *ProviderConfig) validateRequest(apiName ApiName, body []byte) error {
	config := c.requestValidation
	if !config.enabled || apiName != ApiNameChatCompletion {
		return nil
	}

	model := gjson.GetBytes(body, "model")
	if len(config.modelPatterns) > 0 && (!model.Exists() || !config.matchesModel(model.String())) {
		return nil
	}

	var err error
	switch config.profile {
	case requestValidationProfileDeepSeekV4:
		err = (deepSeekRequestValidator{}).Validate(body, config.validateStrictSchema)
	}
	if err == nil {
		return nil
	}

	if config.mode == requestValidationModeShadow {
		if validationErr, ok := err.(*RequestValidationError); ok {
			log.Warnf("[requestValidation] profile=%s rule=%s param=%s mode=shadow", config.profile, validationErr.RuleID, validationErr.Param)
		} else {
			log.Warnf("[requestValidation] profile=%s mode=shadow err=%v", config.profile, err)
		}
		return nil
	}
	return err
}

type deepSeekRequestValidator struct{}

func (deepSeekRequestValidator) Validate(body []byte, validateStrictSchema bool) error {
	if !gjson.ValidBytes(body) {
		return newRequestValidationError(deepSeekRuleInvalidJSON, "body", "Invalid request body format.")
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return newRequestValidationError(deepSeekRuleInvalidJSON, "body", "The request body must be a JSON object.")
	}

	model := root.Get("model")
	if !model.Exists() || model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		return newRequestValidationError(deepSeekRuleModelRequired, "model", "The model field is required and must be a non-empty string.")
	}

	messages := root.Get("messages")
	if !messages.Exists() || !messages.IsArray() || len(messages.Array()) == 0 {
		return newRequestValidationError(deepSeekRuleMessagesRequired, "messages", "Messages must contain at least one item.")
	}

	//if err := validateDeepSeekParameters(root, validateStrictSchema); err != nil {
	//	return err
	//}
	return validateDeepSeekMessages(root)
}

func validateDeepSeekParameters(root gjson.Result, validateStrictSchema bool) error {
	if thinking := root.Get("thinking"); thinking.Exists() && thinking.Type != gjson.Null {
		if !thinking.IsObject() || !isOneOf(thinking.Get("type"), "enabled", "disabled") {
			return newRequestValidationError(deepSeekRuleInvalidParameter, "thinking.type", "thinking.type must be either 'enabled' or 'disabled'.")
		}
	}
	if effort := root.Get("reasoning_effort"); effort.Exists() && effort.Type != gjson.Null && !isOneOf(effort, "low", "medium", "high", "xhigh", "max") {
		return newRequestValidationError(deepSeekRuleInvalidParameter, "reasoning_effort", "reasoning_effort must be one of low, medium, high, xhigh, or max.")
	}
	if responseFormat := root.Get("response_format"); responseFormat.Exists() && responseFormat.Type != gjson.Null {
		if !responseFormat.IsObject() || !isOneOf(responseFormat.Get("type"), "text", "json_object") {
			return newRequestValidationError(deepSeekRuleInvalidParameter, "response_format.type", "response_format.type must be either 'text' or 'json_object'.")
		}
	}
	stream := root.Get("stream")
	if stream.Exists() && stream.Type != gjson.Null && !isJSONBoolean(stream) {
		return newRequestValidationError(deepSeekRuleInvalidParameter, "stream", "stream must be a boolean.")
	}
	if streamOptions := root.Get("stream_options"); streamOptions.Exists() && streamOptions.Type != gjson.Null {
		if !streamOptions.IsObject() || !stream.Bool() {
			return newRequestValidationError(deepSeekRuleInvalidStreamOptions, "stream_options", "stream_options must be an object and may only be set when stream is true.")
		}
		if includeUsage := streamOptions.Get("include_usage"); includeUsage.Exists() && includeUsage.Type != gjson.Null && !isJSONBoolean(includeUsage) {
			return newRequestValidationError(deepSeekRuleInvalidStreamOptions, "stream_options.include_usage", "stream_options.include_usage must be a boolean.")
		}
	}
	if stop := root.Get("stop"); stop.Exists() && stop.Type != gjson.Null {
		if stop.Type == gjson.String {
			// A single stop sequence is valid.
		} else if stop.IsArray() {
			if len(stop.Array()) > 16 {
				return newRequestValidationError(deepSeekRuleInvalidParameter, "stop", "A maximum of 16 stop sequences is supported.")
			}
			for i, item := range stop.Array() {
				if item.Type != gjson.String {
					return newRequestValidationError(deepSeekRuleInvalidParameter, fmt.Sprintf("stop[%d]", i), "Every stop sequence must be a string.")
				}
			}
		} else {
			return newRequestValidationError(deepSeekRuleInvalidParameter, "stop", "stop must be a string or an array of strings.")
		}
	}
	if value := root.Get("temperature"); value.Exists() && value.Type != gjson.Null && (value.Type != gjson.Number || value.Float() < 0 || value.Float() > 2) {
		return newRequestValidationError(deepSeekRuleInvalidParameter, "temperature", "temperature must be a number between 0 and 2.")
	}
	if value := root.Get("top_p"); value.Exists() && value.Type != gjson.Null && (value.Type != gjson.Number || value.Float() < 0 || value.Float() > 1) {
		return newRequestValidationError(deepSeekRuleInvalidParameter, "top_p", "top_p must be a number between 0 and 1.")
	}
	if value := root.Get("top_logprobs"); value.Exists() && value.Type != gjson.Null {
		if value.Type != gjson.Number || math.Trunc(value.Float()) != value.Float() || value.Int() < 0 || value.Int() > 20 || !root.Get("logprobs").Bool() {
			return newRequestValidationError(deepSeekRuleInvalidTopLogprobs, "top_logprobs", "top_logprobs must be between 0 and 20 and requires logprobs=true.")
		}
	}
	if logprobs := root.Get("logprobs"); logprobs.Exists() && logprobs.Type != gjson.Null && !isJSONBoolean(logprobs) {
		return newRequestValidationError(deepSeekRuleInvalidParameter, "logprobs", "logprobs must be a boolean.")
	}
	if userID := root.Get("user_id"); userID.Exists() && userID.Type != gjson.Null && (userID.Type != gjson.String || !deepSeekUserIDPattern.MatchString(userID.String())) {
		return newRequestValidationError(deepSeekRuleInvalidParameter, "user_id", "user_id must contain only letters, digits, underscores, or dashes and be at most 512 characters.")
	}
	return validateDeepSeekTools(root, validateStrictSchema)
}

func validateDeepSeekTools(root gjson.Result, validateStrictSchema bool) error {
	tools := root.Get("tools")
	if !tools.Exists() || tools.Type == gjson.Null {
		return validateDeepSeekToolChoice(root, nil)
	}
	if !tools.IsArray() {
		return newRequestValidationError(deepSeekRuleInvalidToolDefinition, "tools", "tools must be an array.")
	}
	toolList := tools.Array()
	if len(toolList) > 128 {
		return newRequestValidationError(deepSeekRuleTooManyTools, "tools", "A maximum of 128 functions is supported.")
	}

	names := make(map[string]struct{}, len(toolList))
	strictCount := 0
	for i, tool := range toolList {
		base := fmt.Sprintf("tools[%d]", i)
		if !tool.IsObject() || !isOneOf(tool.Get("type"), "function") || !tool.Get("function").IsObject() {
			return newRequestValidationError(deepSeekRuleInvalidToolDefinition, base, "Only function tools are supported.")
		}
		function := tool.Get("function")
		name := function.Get("name")
		if name.Type != gjson.String || !deepSeekFunctionNamePattern.MatchString(name.String()) {
			return newRequestValidationError(deepSeekRuleInvalidFunctionName, base+".function.name", "Function names must match ^[a-zA-Z0-9_-]{1,64}$.")
		}
		if _, exists := names[name.String()]; exists {
			return newRequestValidationError(deepSeekRuleInvalidToolDefinition, base+".function.name", "Function names must be unique within tools.")
		}
		names[name.String()] = struct{}{}
		if description := function.Get("description"); description.Exists() && description.Type != gjson.Null && description.Type != gjson.String {
			return newRequestValidationError(deepSeekRuleInvalidToolDefinition, base+".function.description", "Function descriptions must be strings.")
		}
		if parameters := function.Get("parameters"); parameters.Exists() && parameters.Type != gjson.Null && !parameters.IsObject() {
			return newRequestValidationError(deepSeekRuleInvalidToolDefinition, base+".function.parameters", "Function parameters must be a JSON Schema object.")
		}
		strict := function.Get("strict")
		if strict.Exists() && strict.Type != gjson.Null && !isJSONBoolean(strict) {
			return newRequestValidationError(deepSeekRuleInvalidToolDefinition, base+".function.strict", "Function strict must be a boolean.")
		}
		if strict.Bool() {
			strictCount++
			if validateStrictSchema {
				if err := validateDeepSeekStrictSchema(function.Get("parameters"), base+".function.parameters"); err != nil {
					return err
				}
			}
		}
	}
	if strictCount > 0 && strictCount != len(toolList) {
		return newRequestValidationError(deepSeekRuleInvalidStrictSchema, "tools", "When strict mode is used, every function must set strict=true.")
	}
	return validateDeepSeekToolChoice(root, names)
}

func validateDeepSeekToolChoice(root gjson.Result, toolNames map[string]struct{}) error {
	choice := root.Get("tool_choice")
	if !choice.Exists() || choice.Type == gjson.Null {
		return nil
	}
	if choice.Type == gjson.String {
		if isOneOf(choice, "none", "auto", "required") {
			if len(toolNames) == 0 && choice.String() != "none" {
				return newRequestValidationError(deepSeekRuleInvalidToolChoice, "tool_choice", "tool_choice requires at least one tool unless it is 'none'.")
			}
			return nil
		}
		return newRequestValidationError(deepSeekRuleInvalidToolChoice, "tool_choice", "tool_choice must be none, auto, required, or a named function.")
	}
	if !choice.IsObject() || !isOneOf(choice.Get("type"), "function") {
		return newRequestValidationError(deepSeekRuleInvalidToolChoice, "tool_choice", "A named tool choice must have type 'function'.")
	}
	name := choice.Get("function.name")
	if name.Type != gjson.String {
		return newRequestValidationError(deepSeekRuleInvalidToolChoice, "tool_choice.function.name", "A named tool choice requires a function name.")
	}
	if _, exists := toolNames[name.String()]; !exists {
		return newRequestValidationError(deepSeekRuleInvalidToolChoice, "tool_choice.function.name", "The selected function must be present in tools.")
	}
	return nil
}

func validateDeepSeekMessages(root gjson.Result) error {
	thinkingEnabled := root.Get("thinking.type").String() != "disabled"
	tools := root.Get("tools")
	hasToolsParameter := tools.Exists() && tools.Type != gjson.Null

	pending := make(map[string]struct{})
	allCallIDs := make(map[string]struct{})
	completed := make(map[string]struct{})

	for i, message := range root.Get("messages").Array() {
		base := fmt.Sprintf("messages[%d]", i)
		if !message.IsObject() || message.Get("role").Type != gjson.String {
			return newRequestValidationError(deepSeekRuleInvalidMessage, base, "Each message must be an object with a role.")
		}
		role := message.Get("role").String()
		if role != roleTool && len(pending) > 0 {
			return newRequestValidationError(deepSeekRuleIncompleteToolResults, base, "All preceding tool calls must have a tool response before the next message.")
		}

		switch role {
		case roleSystem:
			if message.Get("content").Type != gjson.String {
				return newRequestValidationError(deepSeekRuleInvalidMessage, base+".content", "System message content must be a string.")
			}
		case roleUser:
			content := message.Get("content")
			if content.Type != gjson.String && !content.IsArray() {
				return newRequestValidationError(deepSeekRuleInvalidMessage, base+".content", "User message content must be a string or an array of content parts.")
			}
		case roleAssistant:
			if thinkingEnabled && hasToolsParameter {
				reasoning := message.Get("reasoning_content")
				if !reasoning.Exists() {
					return newRequestValidationError(deepSeekRuleMissingReasoningContent, base+".reasoning_content", "The `reasoning_content` in thinking mode must be passed back to the API when tools are present.")
				}
				if reasoning.Type != gjson.String && reasoning.Type != gjson.Null {
					return newRequestValidationError(deepSeekRuleInvalidReasoningContent, base+".reasoning_content", "reasoning_content must be a string or null.")
				}
			}
			content := message.Get("content")
			if !content.Exists() || (content.Type != gjson.String && content.Type != gjson.Null) {
				return newRequestValidationError(deepSeekRuleInvalidMessage, base+".content", "Assistant message content must be a string or null.")
			}
			toolCalls := message.Get("tool_calls")
			if toolCalls.Exists() && toolCalls.Type != gjson.Null {
				if !toolCalls.IsArray() || len(toolCalls.Array()) == 0 {
					return newRequestValidationError(deepSeekRuleInvalidToolCall, base+".tool_calls", "tool_calls must be a non-empty array.")
				}
				for j, call := range toolCalls.Array() {
					callBase := fmt.Sprintf("%s.tool_calls[%d]", base, j)
					id := call.Get("id")
					function := call.Get("function")
					name := function.Get("name")
					if id.Type != gjson.String || id.String() == "" || !isOneOf(call.Get("type"), "function") || !function.IsObject() || name.Type != gjson.String || !deepSeekFunctionNamePattern.MatchString(name.String()) || function.Get("arguments").Type != gjson.String {
						return newRequestValidationError(deepSeekRuleInvalidToolCall, callBase, "Each tool call requires an id, type=function, function name, and string arguments.")
					}
					if _, exists := allCallIDs[id.String()]; exists {
						return newRequestValidationError(deepSeekRuleDuplicateToolCall, callBase+".id", "Tool call IDs must be unique.")
					}
					allCallIDs[id.String()] = struct{}{}
					pending[id.String()] = struct{}{}
				}
			}
		case roleTool:
			callID := message.Get("tool_call_id")
			if callID.Type != gjson.String || callID.String() == "" || message.Get("content").Type != gjson.String {
				return newRequestValidationError(deepSeekRuleInvalidMessage, base, "Tool messages require string content and a non-empty tool_call_id.")
			}
			if _, exists := completed[callID.String()]; exists {
				return newRequestValidationError(deepSeekRuleDuplicateToolResponse, base+".tool_call_id", "A tool_call_id may only be answered once.")
			}
			if _, exists := pending[callID.String()]; !exists {
				rule := deepSeekRuleUnknownToolCall
				messageText := "tool_call_id does not match a pending tool call."
				if len(pending) == 0 {
					rule = deepSeekRuleOrphanTool
					messageText = "Messages with role 'tool' must be a response to a preceding message with 'tool_calls'."
				}
				return newRequestValidationError(rule, base+".tool_call_id", messageText)
			}
			delete(pending, callID.String())
			completed[callID.String()] = struct{}{}
		default:
			return newRequestValidationError(deepSeekRuleInvalidRole, base+".role", "Unsupported message role: "+role)
		}
	}
	if len(pending) > 0 {
		return newRequestValidationError(deepSeekRuleIncompleteToolResults, "messages", "Every tool call must have a corresponding tool response.")
	}
	return nil
}

func validateDeepSeekStrictSchema(schema gjson.Result, param string) error {
	if !schema.Exists() || !schema.IsObject() || schema.Get("type").String() != "object" {
		return newRequestValidationError(deepSeekRuleInvalidStrictSchema, param, "Strict functions require a JSON Schema object with type=object.")
	}
	return validateDeepSeekStrictSchemaNode(schema, param)
}

func validateDeepSeekStrictSchemaNode(schema gjson.Result, param string) error {
	if schemaType := schema.Get("type"); schemaType.Exists() && !isOneOf(schemaType, "object", "string", "number", "integer", "boolean", "array") {
		return newRequestValidationError(deepSeekRuleInvalidStrictSchema, param+".type", "Unsupported JSON Schema type in DeepSeek strict mode.")
	}
	for _, keyword := range []string{"minLength", "maxLength", "minItems", "maxItems"} {
		if schema.Get(keyword).Exists() {
			return newRequestValidationError(deepSeekRuleInvalidStrictSchema, param+"."+keyword, keyword+" is not supported in DeepSeek strict mode.")
		}
	}
	if schema.Get("type").String() == "object" {
		properties := schema.Get("properties")
		required := make(map[string]struct{})
		for _, item := range schema.Get("required").Array() {
			required[item.String()] = struct{}{}
		}
		if schema.Get("additionalProperties").Type != gjson.False {
			return newRequestValidationError(deepSeekRuleInvalidStrictSchema, param+".additionalProperties", "Strict object schemas must set additionalProperties=false.")
		}
		for name, property := range properties.Map() {
			if _, ok := required[name]; !ok {
				return newRequestValidationError(deepSeekRuleInvalidStrictSchema, param+".required", "Every object property must be listed in required in strict mode.")
			}
			if err := validateDeepSeekStrictSchemaNode(property, param+".properties."+name); err != nil {
				return err
			}
		}
	}
	if items := schema.Get("items"); items.Exists() && items.IsObject() {
		return validateDeepSeekStrictSchemaNode(items, param+".items")
	}
	for i, variant := range schema.Get("anyOf").Array() {
		if err := validateDeepSeekStrictSchemaNode(variant, fmt.Sprintf("%s.anyOf[%d]", param, i)); err != nil {
			return err
		}
	}
	for name, definition := range schema.Get("$defs").Map() {
		if err := validateDeepSeekStrictSchemaNode(definition, param+".$defs."+name); err != nil {
			return err
		}
	}
	return nil
}

func isJSONBoolean(result gjson.Result) bool {
	return result.Type == gjson.True || result.Type == gjson.False
}

func isOneOf(result gjson.Result, values ...string) bool {
	if result.Type != gjson.String {
		return false
	}
	for _, value := range values {
		if result.String() == value {
			return true
		}
	}
	return false
}
