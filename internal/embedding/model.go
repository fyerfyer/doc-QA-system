package embedding

// TongyiEmbeddingRequest 通义千问嵌入API请求结构
type TongyiEmbeddingRequest struct {
	Model string   `json:"model"`          // 模型名称
	Input []string `json:"input"`          // 需要嵌入的文本列表
	User  string   `json:"user,omitempty"` // 可选的用户标识符
}

// TongyiEmbeddingResponse 通义千问嵌入API响应结构
type TongyiEmbeddingResponse struct {
	Code      int          `json:"code"`       // 响应状态码，0表示成功
	Message   string       `json:"message"`    // 响应消息
	RequestID string       `json:"request_id"` // 请求ID
	Output    TongyiOutput `json:"output"`     // 输出结果
	Usage     TongyiUsage  `json:"usage"`      // 资源使用情况
}

// TongyiOutput 嵌入输出结果
type TongyiOutput struct {
	Embeddings [][]float32 `json:"embeddings"` // 嵌入向量列表
}

// TongyiUsage 资源使用情况
type TongyiUsage struct {
	TotalTokens int `json:"total_tokens"` // 使用的总token数
}
