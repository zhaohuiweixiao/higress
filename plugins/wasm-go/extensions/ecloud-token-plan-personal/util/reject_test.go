package util

import (
	"net/http"
	"testing"

	"ecloud-token-plan-personal/config"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/common"
)

func TestRejectedInfoUsesStandardErrorCodes(t *testing.T) {
	cfg := config.TokenPlanPersonalConfig{
		RejectedCode:               http.StatusTooManyRequests,
		RejectedMsg:                "quota exhausted",
		RejectedNoTokenPlanCode:    http.StatusPaymentRequired,
		RejectedNoTokenPlanMsg:     "subscription required",
		RejectedAgentIDTooLongCode: http.StatusBadRequest,
		RejectedAgentIDTooLongMsg:  "agent id too long",
	}
	tests := []struct {
		reason     RejectReason
		errorType  string
		code       string
		param      any
		statusCode uint32
	}{
		{RejectReasonNoTokenPlan, common.ErrorTypeInvalidRequest, common.ErrorCodeSubscriptionRequired, nil, http.StatusPaymentRequired},
		{RejectReasonQuotaExhausted, common.ErrorTypeInvalidRequest, common.ErrorCodeQuotaExhausted, nil, http.StatusTooManyRequests},
		{RejectReasonAgentIDTooLong, common.ErrorTypeInvalidRequest, common.ErrorCodeInvalidParameter, "agent_id", http.StatusBadRequest},
		{RejectReason("unknown"), common.ErrorTypeAPI, common.ErrorCodeInternalError, nil, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		_, errorType, code, param, statusCode := rejectedInfo(cfg, tt.reason)
		if errorType != tt.errorType || code != tt.code || param != tt.param || statusCode != tt.statusCode {
			t.Fatalf("reason %q returned type=%q code=%q param=%v status=%d", tt.reason, errorType, code, param, statusCode)
		}
	}
}
