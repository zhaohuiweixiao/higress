# AI 网关标准错误码说明

面向客户的统一错误响应规范。

本文档定义 AI 网关首批对外使用的标准错误码，适用于 CodingPlan、Token Plan 和模型调用场景。客户端应依据 `error.code` 处理错误，不应匹配 `message` 文本。

文档状态：正式发布  
版本：V1.0  
更新日期：2026-08-04

> 本版收录 11 个核心错误码。实例、周期和字段维度的细分场景统一通过通用错误码及 `param` 字段表达。

## 错误响应格式

```json
{
  "error": {
    "message": "错误说明",
    "type": "invalid_request_error",
    "param": null,
    "code": "error_code"
  }
}
```

- `code`：稳定的业务错误码，客户端应优先根据该字段处理。
- `message`：面向客户的错误说明，可优化或国际化。
- `type`：错误类别，用于日志归类和监控统计。
- `param`：引发错误的参数；与具体参数无关时为 `null`。

## 一、套餐、额度与限流

### 402-subscription_required

错误码：`subscription_required`  
HTTP 状态码：`402`  
错误类型：`invalid_request_error`  
相关参数：`null`  
错误信息：当前账号没有可用套餐，请完成订购或续费后重试。

**原因**：客户未订购 CodingPlan 或 Token Plan，或者套餐未激活、已到期、已失效。

**解决方案**：订购、续费或重新激活对应套餐，等待状态生效后重新调用。

### 429-quota_exhausted

错误码：`quota_exhausted`  
HTTP 状态码：`429`  
错误类型：`invalid_request_error`  
相关参数：`null`  
错误信息：当前配额已用尽，请稍后再试或扩充配额。

**原因**：有效套餐的请求额度、Token 额度或算力额度已经用完。

**解决方案**：等待配额周期重置，或者升级、扩充当前套餐。

### 429-rate_limit_exceeded

错误码：`rate_limit_exceeded`  
HTTP 状态码：`429`  
错误类型：`rate_limit_error`  
相关参数：`null`  
错误信息：Rate limit exceeded. Please try again later.

**原因**：单位时间内的请求数量超过平台允许的频率限制。

**解决方案**：降低请求频率，并根据响应头 `Retry-After` 或限流重置时间重试。

> 实例级和 API Key 级额度耗尽统一返回 `quota_exhausted`；实例级频率限制统一返回 `rate_limit_exceeded`。

## 二、API Key 与访问权限

### 400-missing_api_key

错误码：`missing_api_key`  
HTTP 状态码：`400`  
错误类型：`invalid_request_error`  
相关参数：`api_key`  
错误信息：API key is required.

**原因**：请求没有携带 API Key，或者认证请求头为空。

**解决方案**：在请求头中携带平台签发的 API Key，并检查请求头名称和认证格式。

### 403-api_key_not_assigned

错误码：`api_key_not_assigned`  
HTTP 状态码：`403`  
错误类型：`invalid_request_error`  
相关参数：`api_key`  
错误信息：当前 API Key 未绑定可用套餐或实例。

**原因**：API Key 存在，但没有绑定可用的 CodingPlan、Token Plan 或对应实例。

**解决方案**：为 API Key 分配套餐或实例，或者更换已经完成绑定的 API Key。

### 401-invalid_api_key

错误码：`invalid_api_key`  
HTTP 状态码：`401`  
错误类型：`authentication_error`  
相关参数：`api_key`  
错误信息：Invalid API key provided.

**原因**：API Key 错误、格式不合法、已经过期或已被撤销。

**解决方案**：检查 API Key 是否完整并属于当前环境；必要时重新创建 API Key。

## 三、请求格式与参数

### 400-invalid_request_body

错误码：`invalid_request_body`  
HTTP 状态码：`400`  
错误类型：`invalid_request_error`  
相关参数：`null`  
错误信息：请求体格式不正确，请检查 JSON 格式。

**原因**：请求体不是合法 JSON、字段层级错误，或者服务无法解析请求内容。

**解决方案**：检查 JSON 的括号、引号、逗号和字段结构，确保请求体符合接口定义。

### 400-invalid_parameter

错误码：`invalid_parameter`  
HTTP 状态码：`400`  
错误类型：`invalid_request_error`  
相关参数：具体字段名  
错误信息：参数不合法，请根据 `param` 字段检查请求。

**原因**：参数类型、格式、长度、枚举值或取值范围不符合接口要求。

**解决方案**：根据 `error.param` 和 `message` 修正对应字段后重新提交。

> 具体参数问题不再创建独立错误码。例如 `agent_id` 过长时返回 `invalid_parameter`，`param` 为 `"agent_id"`。

## 四、模型与资源相关

### 400-model_not_supported

错误码：`model_not_supported`  
HTTP 状态码：`400`  
错误类型：`invalid_request_error`  
相关参数：`model`  
错误信息：Model is not supported.

**原因**：模型存在，但当前套餐、调用协议或接口能力不支持该模型。

**解决方案**：切换到当前套餐和接口支持的模型，或者调整调用方式。

### 404-resource_not_found

错误码：`resource_not_found`<br>
HTTP 状态码：`404`  
错误类型：`invalid_request_error`  
相关参数：`null`<br>
错误信息：请求的资源不存在，请检查请求地址或参数。

**原因**：请求的模型、路由或其他资源不存在，或者当前调用环境中无法访问该资源。

**解决方案**：检查模型名称、请求地址、接口路径、地域和调用环境是否正确。

## 五、平台内部错误

### 500-internal_error

错误码：`internal_error`  
HTTP 状态码：`500`  
错误类型：`api_error`  
相关参数：`null`  
错误信息：系统繁忙，请稍后重试。

**原因**：AI 网关发生无法进一步分类的内部异常。

**解决方案**：使用指数退避策略稍后重试；若持续出现，请携带请求 ID 和发生时间联系技术支持。

## 典型响应示例

### 套餐配额用尽

```json
{
  "error": {
    "message": "当前配额已用尽，请稍后再试或扩充配额。",
    "type": "invalid_request_error",
    "param": null,
    "code": "quota_exhausted"
  }
}
```

### 请求频率超限

```json
{
  "error": {
    "message": "Rate limit exceeded. Please try again later.",
    "type": "rate_limit_error",
    "param": null,
    "code": "rate_limit_exceeded"
  }
}
```

## 客户端处理规范

- 收到 `400`、`401`、`402`、`403` 或 `404` 时，应先修正请求、认证信息、套餐状态或权限配置。
- 收到 `429` 时，应根据 `Retry-After` 或限流重置时间重试。
- 收到 `500` 时，可使用指数退避策略重试。
- 客户端必须依据 `error.code` 编写处理逻辑，不得依赖 `message` 文本。
