package mcp

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ── ToolHandler 工具执行函数签名 ──
// 接收参数 JSON，返回结果文本
type ToolHandler func(args json.RawMessage) (*CallToolResult, error)

// ── ResourceHandler 资源读取函数签名 ──
type ResourceHandler func(uri string) (*ReadResourceResult, error)

// ── MCPServer MCP 协议引擎 ──
type MCPServer struct {
	mu      sync.RWMutex
	name    string
	version string

	tools        map[string]Tool
	toolHandlers map[string]ToolHandler

	resources        map[string]Resource
	resourceHandlers map[string]ResourceHandler

	prompts []Prompt

	initialized bool
	clientInfo  *ClientInfo
}

// NewMCPServer 创建 MCP 服务端实例
func NewMCPServer(name, version string) *MCPServer {
	return &MCPServer{
		name:             name,
		version:          version,
		tools:            make(map[string]Tool),
		toolHandlers:     make(map[string]ToolHandler),
		resources:        make(map[string]Resource),
		resourceHandlers: make(map[string]ResourceHandler),
	}
}

// RegisterTool 注册工具及其处理函数
func (s *MCPServer) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.toolHandlers[tool.Name] = handler
}

// RegisterResource 注册资源及其处理函数
func (s *MCPServer) RegisterResource(resource Resource, handler ResourceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[resource.URI] = resource
	s.resourceHandlers[resource.URI] = handler
}

// RegisterPrompt 注册提示词模板
func (s *MCPServer) RegisterPrompt(prompt Prompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// ── JSON-RPC 消息分发 ──

// HandleMessage 处理 JSON-RPC 消息，返回响应（nil 表示通知无需响应）
func (s *MCPServer) HandleMessage(raw json.RawMessage) *JSONRPCResponse {
	// 尝试解析为请求
	var req JSONRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return s.errorResponse(nil, ErrParseError, "Parse error: "+err.Error())
	}

	// 通知（无 id）不返回响应
	if req.ID == nil {
		s.handleNotification(&req)
		return nil
	}

	return s.handleRequest(&req)
}

func (s *MCPServer) handleRequest(req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	case "prompts/list":
		return s.handlePromptsList(req)
	case "prompts/get":
		return s.handlePromptsGet(req)
	case "ping":
		return s.handlePing(req)
	default:
		resp := JSONRPCErrorResponse(req.ID, ErrMethodNotFound, fmt.Sprintf("Method not found: %s", req.Method))
		return &resp
	}
}

func (s *MCPServer) handleNotification(req *JSONRPCRequest) {
	switch req.Method {
	case "notifications/initialized":
		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()
	case "notifications/cancelled":
		// 取消通知，当前版本暂不处理
	}
}

// ── initialize ──

func (s *MCPServer) handleInitialize(req *JSONRPCRequest) *JSONRPCResponse {
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		resp := JSONRPCErrorResponse(req.ID, ErrInvalidParams, "Invalid initialize params: "+err.Error())
		return &resp
	}

	if !IsVersionSupported(params.ProtocolVersion) {
		resp := JSONRPCErrorResponse(req.ID, ErrInvalidParams,
			fmt.Sprintf("Unsupported protocol version: %s. Supported: %v", params.ProtocolVersion, SupportedVersions()))
		return &resp
	}

	s.mu.Lock()
	s.clientInfo = &params.ClientInfo
	s.mu.Unlock()

	// 协商版本：使用客户端请求的版本（若受支持）
	negotiatedVersion := params.ProtocolVersion

	result := InitializeResult{
		ProtocolVersion: negotiatedVersion,
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{ListChanged: false},
			Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
			Prompts:   &PromptsCapability{ListChanged: false},
			Logging:   &LoggingCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    s.name,
			Version: s.version,
		},
		Instructions: s.getInstructions(),
	}

	resp := JSONRPCSuccessResponse(req.ID, result)
	return &resp
}

func (s *MCPServer) getInstructions() string {
	return `edgeCore MCP Server — 工业边缘网关协议操作接口

# 可用能力
- **Tools**: 查询通道/设备/点位、读写点位值、分析协议报文、获取诊断信息
- **Resources**: 结构化数据资源（通道列表、设备详情、点位配置、系统状态）
- **Prompts**: 常用工业协议分析提示词模板

# 接入方式
将此 MCP Server 配置到你的 LLM 客户端（如 Claude Desktop、Cursor 等）：
{
  "mcpServers": {
    "edgeCore": {
      "url": "http://<edgeCore-host>:8080/api/mcp"
    }
  }
}

# 安全说明
- 写操作（write_point）需要人工确认，不会自动执行
- 所有操作通过 edgeCore JWT 认证
- 敏感配置信息已脱敏处理`
}

// ── tools/list ──

func (s *MCPServer) handleToolsList(req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, t)
	}

	resp := JSONRPCSuccessResponse(req.ID, ListToolsResult{Tools: tools})
	return &resp
}

// ── tools/call ──

func (s *MCPServer) handleToolsCall(req *JSONRPCRequest) (resp *JSONRPCResponse) {
	// Panic recovery: prevents a single tool handler crash from killing the
	// HTTP connection and cascading into SSE stream termination.
	// panic 恢复：防止单个工具处理器崩溃导致 HTTP 连接中断和 SSE 流终止。
	defer func() {
		if r := recover(); r != nil {
			errResp := JSONRPCErrorResponse(req.ID, ErrInternalError, fmt.Sprintf("tool execution panic: %v", r))
			resp = &errResp
		}
	}()

	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		errResp := JSONRPCErrorResponse(req.ID, ErrInvalidParams, "Invalid tool call params: "+err.Error())
		return &errResp
	}

	s.mu.RLock()
	handler, ok := s.toolHandlers[params.Name]
	s.mu.RUnlock()

	if !ok {
		errResp := JSONRPCErrorResponse(req.ID, ErrMethodNotFound, fmt.Sprintf("Tool not found: %s", params.Name))
		return &errResp
	}

	result, err := handler(params.Arguments)
	if err != nil {
		result = NewErrorResult(err.Error())
	}

	successResp := JSONRPCSuccessResponse(req.ID, result)
	return &successResp
}

// ── resources/list ──

func (s *MCPServer) handleResourcesList(req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]Resource, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, r)
	}

	resp := JSONRPCSuccessResponse(req.ID, ListResourcesResult{Resources: resources})
	return &resp
}

// ── resources/read ──

func (s *MCPServer) handleResourcesRead(req *JSONRPCRequest) *JSONRPCResponse {
	var params ReadResourceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		resp := JSONRPCErrorResponse(req.ID, ErrInvalidParams, "Invalid resource read params: "+err.Error())
		return &resp
	}

	s.mu.RLock()
	handler, ok := s.resourceHandlers[params.URI]
	s.mu.RUnlock()

	if !ok {
		resp := JSONRPCErrorResponse(req.ID, ErrMethodNotFound, fmt.Sprintf("Resource not found: %s", params.URI))
		return &resp
	}

	result, err := handler(params.URI)
	if err != nil {
		resp := JSONRPCErrorResponse(req.ID, ErrInternalError, err.Error())
		return &resp
	}

	resp := JSONRPCSuccessResponse(req.ID, result)
	return &resp
}

// ── prompts/list ──

func (s *MCPServer) handlePromptsList(req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := JSONRPCSuccessResponse(req.ID, ListPromptsResult{Prompts: s.prompts})
	return &resp
}

// ── prompts/get ──

func (s *MCPServer) handlePromptsGet(req *JSONRPCRequest) *JSONRPCResponse {
	var params GetPromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		resp := JSONRPCErrorResponse(req.ID, ErrInvalidParams, "Invalid prompt get params: "+err.Error())
		return &resp
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.prompts {
		if p.Name == params.Name {
			// 构建提示词消息
			promptText := fmt.Sprintf("# %s\n\n%s", p.Name, p.Description)
			result := GetPromptResult{
				Description: p.Description,
				Messages: []PromptMessage{
					{Role: "user", Content: NewTextContent(promptText)},
				},
			}
			resp := JSONRPCSuccessResponse(req.ID, result)
			return &resp
		}
	}

	resp := JSONRPCErrorResponse(req.ID, ErrMethodNotFound, fmt.Sprintf("Prompt not found: %s", params.Name))
	return &resp
}

// ── ping ──

func (s *MCPServer) handlePing(req *JSONRPCRequest) *JSONRPCResponse {
	resp := JSONRPCSuccessResponse(req.ID, map[string]string{"status": "ok"})
	return &resp
}

// ── 辅助 ──

func (s *MCPServer) errorResponse(id any, code int, message string) *JSONRPCResponse {
	resp := JSONRPCErrorResponse(id, code, message)
	return &resp
}

// IsInitialized 检查是否已完成初始化握手
func (s *MCPServer) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// GetTools 返回所有已注册工具
func (s *MCPServer) GetTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetResources 返回所有已注册资源
func (s *MCPServer) GetResources() []Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resources := make([]Resource, 0, len(s.resources))
	for _, r := range s.resources {
		resources = append(resources, r)
	}
	return resources
}

// GetPrompts 返回所有已注册提示词
func (s *MCPServer) GetPrompts() []Prompt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prompts
}

// GetToolNames 返回所有工具名称
func (s *MCPServer) GetToolNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.tools))
	for _, t := range s.tools {
		names = append(names, t.Name)
	}
	return names
}

// GetResourceURIs 返回所有资源 URI
func (s *MCPServer) GetResourceURIs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uris := make([]string, 0, len(s.resources))
	for _, r := range s.resources {
		uris = append(uris, r.URI)
	}
	return uris
}

// GetPromptNames 返回所有提示词名称
func (s *MCPServer) GetPromptNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.prompts))
	for _, p := range s.prompts {
		names = append(names, p.Name)
	}
	return names
}
