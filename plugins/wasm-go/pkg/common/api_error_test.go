package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAPIErrorBody(t *testing.T) {
	body := BuildAPIErrorBody(
		"API key is required.",
		ErrorTypeInvalidRequest,
		"api_key",
		ErrorCodeMissingAPIKey,
	)

	var response APIErrorResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, "API key is required.", response.Error.Message)
	require.Equal(t, ErrorTypeInvalidRequest, response.Error.Type)
	require.Equal(t, "api_key", response.Error.Param)
	require.Equal(t, ErrorCodeMissingAPIKey, response.Error.Code)
}

func TestBuildAPIErrorBodyWithNilParam(t *testing.T) {
	body := BuildAPIErrorBody(
		"当前配额已用尽。",
		ErrorTypeInvalidRequest,
		nil,
		ErrorCodeQuotaExhausted,
	)

	var response APIErrorResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Nil(t, response.Error.Param)
}

func TestBuildResourceNotFoundAPIErrorBody(t *testing.T) {
	body := BuildAPIErrorBody(
		"请求的资源不存在，请检查请求地址或参数。",
		ErrorTypeInvalidRequest,
		nil,
		ErrorCodeResourceNotFound,
	)

	var response APIErrorResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, ErrorCodeResourceNotFound, response.Error.Code)
	require.Equal(t, ErrorTypeInvalidRequest, response.Error.Type)
	require.Nil(t, response.Error.Param)
}

func TestBuildAPIErrorBodyFallsBackToInternalError(t *testing.T) {
	body := BuildAPIErrorBody("bad", ErrorTypeInvalidRequest, make(chan int), "bad")

	var response APIErrorResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, ErrorCodeInternalError, response.Error.Code)
	require.Equal(t, ErrorTypeAPI, response.Error.Type)
	require.Nil(t, response.Error.Param)
}
