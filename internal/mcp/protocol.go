// Package mcp — Model Context Protocol (MCP) 服务端实现
// 提供 JSON-RPC 2.0 协议处理，允许外部 LLM 应用通过标准 MCP 协议操作 edgeCore
package mcp

import "encoding/json"

// ── JSON-RPC 2.0 消息类型 ──

// JSONRPCRequest 标准 JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse 标准 JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCNotification 标准 JSON-RPC 通知（无 id，无需响应）
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCError JSON-RPC 错误
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ── MCP 协议常量 ──

const (
	MCPVersion    = "2024-11-05"
	MCPVersionV25 = "2025-11-25"
	ProtocolName  = "mcp"
	ServerName    = "edgeCore-MCP-Server"
	ServerVersion = "1.0.0"

	// JSON-RPC 标准错误码
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternalError  = -32603
)

// ── MCP 初始化 ──

// InitializeParams initialize 请求参数
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// ClientCapabilities 客户端能力声明
type ClientCapabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

// RootsCapability roots 能力
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability sampling 能力
type SamplingCapability struct{}

// ClientInfo 客户端信息
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult initialize 响应
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ServerCapabilities 服务端能力声明
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Logging   *LoggingCapability   `json:"logging,omitempty"`
}

// ToolsCapability 工具能力
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability 资源能力
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability 提示词能力
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability 日志能力
type LoggingCapability struct{}

// ServerInfo 服务端信息
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ── MCP 工具 (Tools) ──

// ListToolsResult tools/list 响应
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// Tool MCP 工具定义
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema JSON Schema 输入定义
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]PropertyDef `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

// PropertyDef 属性定义
type PropertyDef struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// CallToolParams tools/call 请求参数
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult tools/call 响应
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ── MCP 资源 (Resources) ──

// ListResourcesResult resources/list 响应
type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// Resource MCP 资源定义
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ReadResourceParams resources/read 请求参数
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// ReadResourceResult resources/read 响应
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent 资源内容
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// ── MCP 提示词 (Prompts) ──

// ListPromptsResult prompts/list 响应
type ListPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
}

// Prompt MCP 提示词定义
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument 提示词参数
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// GetPromptParams prompts/get 请求参数
type GetPromptParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// GetPromptResult prompts/get 响应
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage 提示词消息
type PromptMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

// ── 通用内容块 ──

// ContentBlock 内容块（文本/图片/资源）
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
}

// ── 辅助函数 ──

// SupportedVersions 返回所有支持的协议版本
func SupportedVersions() []string {
	return []string{MCPVersion, MCPVersionV25}
}

// IsVersionSupported 检查协议版本是否受支持
func IsVersionSupported(v string) bool {
	for _, sv := range SupportedVersions() {
		if v == sv {
			return true
		}
	}
	return false
}

// NewTextContent 创建文本内容块
func NewTextContent(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// NewErrorResult 创建错误结果
func NewErrorResult(errMsg string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentBlock{NewTextContent("Error: " + errMsg)},
		IsError: true,
	}
}

// NewSuccessResult 创建成功结果
func NewSuccessResult(text string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentBlock{NewTextContent(text)},
	}
}

// JSONRPCErrorResponse 创建 JSON-RPC 错误响应
func JSONRPCErrorResponse(id any, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
}

// JSONRPCSuccessResponse 创建 JSON-RPC 成功响应
func JSONRPCSuccessResponse(id any, result any) JSONRPCResponse {
	raw, _ := json.Marshal(result)
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  raw,
	}
}
