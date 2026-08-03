package common

import "encoding/json"

const (
	ErrorTypeInvalidRequest = "invalid_request_error"
	ErrorTypeRateLimit      = "rate_limit_error"
	ErrorTypeAuthentication = "authentication_error"
	ErrorTypeAPI            = "api_error"
)

const (
	ErrorCodeSubscriptionRequired = "subscription_required"
	ErrorCodeQuotaExhausted       = "quota_exhausted"
	ErrorCodeRateLimitExceeded    = "rate_limit_exceeded"
	ErrorCodeMissingAPIKey        = "missing_api_key"
	ErrorCodeAPIKeyNotAssigned    = "api_key_not_assigned"
	ErrorCodeInvalidAPIKey        = "invalid_api_key"
	ErrorCodeInvalidRequestBody   = "invalid_request_body"
	ErrorCodeInvalidParameter     = "invalid_parameter"
	ErrorCodeModelNotSupported    = "model_not_supported"
	ErrorCodeModelNotFound        = "model_not_found"
	ErrorCodeResourceNotFound     = "resource_not_found"
	ErrorCodeInternalError        = "internal_error"
)

const InternalErrorMessage = "系统繁忙，请稍后重试。"

type APIErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   any    `json:"param"`
	Code    string `json:"code"`
}

type APIErrorResponse struct {
	Error APIErrorDetail `json:"error"`
}

func BuildAPIErrorBody(message, errorType string, param any, code string) []byte {
	body, err := json.Marshal(APIErrorResponse{
		Error: APIErrorDetail{
			Message: message,
			Type:    errorType,
			Param:   param,
			Code:    code,
		},
	})
	if err == nil {
		return body
	}
	return []byte(`{"error":{"message":"系统繁忙，请稍后重试。","type":"api_error","param":null,"code":"internal_error"}}`)
}

func BuildInternalErrorBody() []byte {
	return BuildAPIErrorBody(InternalErrorMessage, ErrorTypeAPI, nil, ErrorCodeInternalError)
}
