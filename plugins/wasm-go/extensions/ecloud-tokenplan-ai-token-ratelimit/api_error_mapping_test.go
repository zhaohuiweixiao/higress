package main

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestRejectedErrorUsesStandardErrorCodes(t *testing.T) {
	tests := []struct {
		name      string
		reason    RejectReason
		errorType string
		code      string
		param     any
	}{
		{"subscription required", RejectReasonNoTokenPlan, common.ErrorTypeInvalidRequest, common.ErrorCodeSubscriptionRequired, nil},
		{"missing api key", RejectReasonMissingAPIKey, common.ErrorTypeInvalidRequest, common.ErrorCodeMissingAPIKey, "api_key"},
		{"api key not assigned", RejectReasonAPIKeyNotAssigned, common.ErrorTypeInvalidRequest, common.ErrorCodeAPIKeyNotAssigned, "api_key"},
		{"minute rate limit", RejectReasonInstanceMinute, common.ErrorTypeRateLimit, common.ErrorCodeRateLimitExceeded, nil},
		{"instance quota", RejectReasonInstanceMonth, common.ErrorTypeInvalidRequest, common.ErrorCodeQuotaExhausted, nil},
		{"api key quota", RejectReasonAPIKeyMonth, common.ErrorTypeInvalidRequest, common.ErrorCodeQuotaExhausted, nil},
		{"generic quota", RejectReasonQuotaExhausted, common.ErrorTypeInvalidRequest, common.ErrorCodeQuotaExhausted, nil},
		{"invalid agent id", RejectReasonAgentIDTooLong, common.ErrorTypeInvalidRequest, common.ErrorCodeInvalidParameter, "agent_id"},
		{"unknown reason", RejectReason("unknown"), common.ErrorTypeAPI, common.ErrorCodeInternalError, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorType, code, param := rejectedError(tt.reason)
			require.Equal(t, tt.errorType, errorType)
			require.Equal(t, tt.code, code)
			require.Equal(t, tt.param, param)
		})
	}
}
