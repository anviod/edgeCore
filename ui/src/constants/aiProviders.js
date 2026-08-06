/** AI 助手设置三大分类 */
export const AI_SETTING_CATEGORIES = [
  { value: 'mcp', label: 'MCP 接入', desc: '外部 LLM 通过 MCP 协议操作网关' },
  { value: 'remote', label: '本地 AI 模型', desc: '局域网 gRPC 对接 edgeCore AI Model Center' },
  { value: 'cloud', label: '云端大模型', desc: '直连公网或私有 LLM API' }
]

/** AI deployment modes (legacy — kept for compatibility) */
export const AI_DEPLOYMENT_MODES = [
  { value: 'remote', label: '本地 AI 模型', desc: '局域网 gRPC 对接 edgeCore AI Model Center（Mode A/B）' },
  { value: 'cloud', label: '云端大模型', desc: '直连公网或私有 LLM API（Mode C，需 enable_cloud）' }
]

export const AI_AUTH_TYPES = [
  { value: 'none', label: '无认证' },
  { value: 'bearer', label: 'Bearer Token', desc: 'Authorization: Bearer <token>' },
  { value: 'api_key', label: 'API Key Header', desc: '自定义 Header 传递密钥' },
  { value: 'basic', label: 'Basic Auth', desc: 'HTTP Basic 用户名/密码' },
  { value: 'azure_key', label: 'Azure API Key', desc: 'api-key Header（Azure OpenAI）' },
  { value: 'custom_header', label: '自定义 Header', desc: '键值对形式附加请求头' }
]

export const AI_PROVIDERS = [
  {
    value: 'edgeCore-center',
    label: 'edgeCore AI Model Center',
    deploymentMode: 'remote',
    authType: 'none',
    baseUrl: '',
    grpcEndpoint: '127.0.0.1:50051',
    models: ['copilot-service', 'protocol-reverse-v1']
  },
  {
    value: 'openai',
    label: 'OpenAI',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: 'https://api.openai.com/v1',
    apiKeyHeader: 'Authorization',
    models: ['gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo', 'o1-mini']
  },
  {
    value: 'anthropic',
    label: 'Anthropic',
    deploymentMode: 'cloud',
    authType: 'api_key',
    baseUrl: 'https://api.anthropic.com',
    apiKeyHeader: 'x-api-key',
    models: ['claude-sonnet-4-20250514', 'claude-3-5-sonnet-20241022', 'claude-3-haiku-20240307']
  },
  {
    value: 'azure-openai',
    label: 'Azure OpenAI',
    deploymentMode: 'cloud',
    authType: 'azure_key',
    baseUrl: 'https://{resource}.openai.azure.com',
    apiKeyHeader: 'api-key',
    azureApiVersion: '2024-02-15-preview',
    models: ['gpt-4o', 'gpt-4', 'gpt-35-turbo']
  },
  {
    value: 'deepseek',
    label: 'DeepSeek',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: 'https://api.deepseek.com/v1',
    models: ['deepseek-chat', 'deepseek-reasoner']
  },
  {
    value: 'qwen',
    label: '通义千问（DashScope）',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    models: ['qwen-plus', 'qwen-turbo', 'qwen-max']
  },
  {
    value: 'ernie',
    label: '文心一言（百度）',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: 'https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop',
    models: ['ernie-4.0-8k', 'ernie-3.5-8k', 'ernie-speed-128k']
  },
  {
    value: 'zhipu',
    label: '智谱 AI',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    models: ['glm-4-plus', 'glm-4-flash', 'glm-4-air']
  },
  {
    value: 'moonshot',
    label: 'Moonshot / Kimi',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: 'https://api.moonshot.cn/v1',
    models: ['moonshot-v1-8k', 'moonshot-v1-32k', 'moonshot-v1-128k']
  },
  {
    value: 'custom',
    label: '自定义',
    deploymentMode: 'cloud',
    authType: 'bearer',
    baseUrl: '',
    models: []
  }
]

export const defaultAiSettings = () => ({
  deployment_mode: 'remote',
  provider: 'edgeCore-center',
  grpc_endpoint: '127.0.0.1:50051',
  base_url: '',
  auth_type: 'bearer',
  api_key: '',
  api_key_set: false,
  api_key_header: 'X-API-Key',
  username: '',
  password: '',
  password_set: false,
  azure_api_version: '2024-02-15-preview',
  custom_headers: {},
  model: '',
  enable_cloud: false,
  tokens_limit: 50000,
  tasks_limit: 100,
  mcp_enabled: false,
  mcp_api_key: '',
  mcp_api_key_set: false,
  mcp_full_access: false
})

export function findProvider(value) {
  return AI_PROVIDERS.find((p) => p.value === value) || null
}

export function applyProviderPreset(form, providerValue) {
  const preset = findProvider(providerValue)
  if (!preset) return
  form.provider = preset.value
  form.deployment_mode = preset.deploymentMode
  if (preset.grpcEndpoint !== undefined) form.grpc_endpoint = preset.grpcEndpoint
  if (preset.baseUrl !== undefined) form.base_url = preset.baseUrl
  if (preset.authType) form.auth_type = preset.authType
  if (preset.apiKeyHeader) form.api_key_header = preset.apiKeyHeader
  if (preset.azureApiVersion) form.azure_api_version = preset.azureApiVersion
  if (preset.models?.length && !form.model) form.model = preset.models[0]
}
