package main

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/common"
)

func TestRejectedErrorUsesStandardErrorCodes(t *testing.T) {
	tests := []struct {
		name      string
		reason    RejectReason
		errorType string
		code      string
	}{
		{"subscription required", RejectReasonNoCodingPlan, common.ErrorTypeInvalidRequest, common.ErrorCodeSubscriptionRequired},
		{"quota exhausted", RejectReasonQuotaExhausted, common.ErrorTypeInvalidRequest, common.ErrorCodeQuotaExhausted},
		{"unknown reason", RejectReason("unknown"), common.ErrorTypeAPI, common.ErrorCodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errorType, code := rejectedError(tt.reason, "configured message")
			if errorType != tt.errorType {
				t.Fatalf("error type = %q, want %q", errorType, tt.errorType)
			}
			if code != tt.code {
				t.Fatalf("error code = %q, want %q", code, tt.code)
			}
		})
	}
}
