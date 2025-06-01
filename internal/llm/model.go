package llm

import "time"

// MessageRole 消息角色类型
type MessageRole string

const (
	// RoleSystem 系统角色
	RoleSystem MessageRole = "system"
	// RoleUser 用户角色
	RoleUser MessageRole = "user"
	// RoleAssistant 助手角色
	RoleAssistant MessageRole = "assistant"
	// RoleTool 工具角色
	RoleTool MessageRole = "tool"
)

// Message 对话消息结构
type Message struct {
	Role    MessageRole `json:"role"`           // 角色
	Content string      `json:"content"`        // 内容
	Name    string      `json:"name,omitempty"` // 可选名称标识
}

// TongyiRequest 通义千问请求结构
type TongyiRequest struct {
	Model      string              `json:"model"`                // 模型名称
	Input      *TongyiRequestInput `json:"input"`                // 输入内容
	Parameters *TongyiParameters   `json:"parameters,omitempty"` // 可选参数
}

// TongyiRequestInput 请求输入内容
type TongyiRequestInput struct {
	Messages []Message `json:"messages"` // 消息列表
}

// TongyiParameters 请求参数
type TongyiParameters struct {
	Temperature       *float32 `json:"temperature,omitempty"`        // 采样温度
	TopP              *float32 `json:"top_p,omitempty"`              // 核采样概率阈值
	TopK              *int     `json:"top_k,omitempty"`              // 生成候选集大小
	MaxTokens         *int     `json:"max_tokens,omitempty"`         // 最大生成Token数
	Seed              *int     `json:"seed,omitempty"`               // 随机数种子
	RepetitionPenalty *float32 `json:"repetition_penalty,omitempty"` // 重复惩罚系数
	PresencePenalty   *float32 `json:"presence_penalty,omitempty"`   // 内容重复度控制
	ResultFormat      string   `json:"result_format,omitempty"`      // 返回格式，message或text
	Stream            bool     `json:"stream,omitempty"`             // 是否流式输出
	IncrementalOutput bool     `json:"incremental_output,omitempty"` // 是否增量输出
}

// TongyiResponse 通义千问响应结构
type TongyiResponse struct {
	StatusCode int          `json:"status_code"` // 状态码
	RequestID  string       `json:"request_id"`  // 请求ID
	Code       string       `json:"code"`        // 错误码(如果有)
	Message    string       `json:"message"`     // 错误消息(如果有)
	Output     TongyiOutput `json:"output"`      // 输出结果
	Usage      TongyiUsage  `json:"usage"`       // 资源使用情况
}

// TongyiOutput 输出结构
type TongyiOutput struct {
	Text         *string        `json:"text"`          // 文本输出(当result_format为text时)
	FinishReason *string        `json:"finish_reason"` // 结束原因
	Choices      []TongyiChoice `json:"choices"`       // 选择列表(当result_format为message时)
}

// TongyiChoice 输出选择
type TongyiChoice struct {
	FinishReason string  `json:"finish_reason"` // 结束原因
	Message      Message `json:"message"`       // 消息内容
}

// TongyiUsage 资源使用情况
type TongyiUsage struct {
	InputTokens  int `json:"input_tokens"`  // 输入token数
	OutputTokens int `json:"output_tokens"` // 输出token数
	TotalTokens  int `json:"total_tokens"`  // 总token数
}

// ChatCompletionRequest 通用聊天请求结构
type ChatCompletionRequest struct {
	Model       string    // 模型名称
	Messages    []Message // 对话历史消息
	MaxTokens   int       // 最大生成Token数
	Temperature float32   // 采样温度
	TopP        float32   // 核采样概率阈值
	Stream      bool      // 是否流式输出
}

// Response 统一的响应结构
type Response struct {
	Text       string    // 生成的文本
	Messages   []Message // 消息列表（如果是对话）
	TokenCount int       // 使用的token数
	ModelName  string    // 使用的模型名称
	FinishTime time.Time // 完成时间
	Error      error     // 如果出错，则包含错误信息
}

// RAGResponse RAG响应结构
type RAGResponse struct {
	Answer  string            // 回答内容
	Sources []SourceReference // 引用来源
}

// SourceReference 引用来源
type SourceReference struct {
	ID       string                 // 文档ID
	FileID   string                 // 文件ID
	FileName string                 // 文件名
	Content  string                 // 引用内容
	Metadata map[string]interface{} // 元数据
}

// Model 常用模型名称
const (
	ModelQwenTurbo  = "qwen-turbo"   // 通义千问-Turbo模型（较快，基础能力）
	ModelQwenPlus   = "qwen-plus"    // 通义千问-Plus模型（平衡速度和性能）
	ModelQwenMax    = "qwen-max"     // 通义千问-Max模型（高级能力，速度较慢）
	ModelQwenLong   = "qwen-long"    // 通义千问-Long模型（支持长上下文）
	ModelQwenVLPlus = "qwen-vl-plus" // 通义千问VL-Plus模型（支持图像）
	ModelDeepSeek   = "deepseek"     // DeepSeek模型
)
