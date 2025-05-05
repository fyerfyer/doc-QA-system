package model

import (
	"time"
)

// CreateChatRequest 创建聊天会话请求
type CreateChatRequest struct {
	Title string `json:"title,omitempty"` // 会话标题，可选，如果不提供将使用默认标题
}

// CreateMessageRequest 创建聊天消息请求
type CreateMessageRequest struct {
	SessionID string                 `json:"session_id" binding:"required"` // 会话ID
	Role      string                 `json:"role" binding:"required"`       // 消息角色：user, system, assistant
	Content   string                 `json:"content" binding:"required"`    // 消息内容
	Metadata  map[string]interface{} `json:"metadata,omitempty"`            // 消息元数据，可选
}

// GetChatHistoryRequest 获取聊天历史请求
type GetChatHistoryRequest struct {
	SessionID         string `uri:"session_id" binding:"required"` // 会话ID
	PaginationRequest        // 嵌入分页请求
}

// ChatListRequest 聊天会话列表请求
type ChatListRequest struct {
	PaginationRequest            // 嵌入分页请求
	StartTime         *time.Time `form:"start_time" json:"start_time,omitempty"` // 开始时间
	EndTime           *time.Time `form:"end_time" json:"end_time,omitempty"`     // 结束时间
	Tags              string     `form:"tags" json:"tags,omitempty"`             // 标签过滤
}

// RenameChatRequest 重命名聊天会话请求
type RenameChatRequest struct {
	SessionID string `uri:"session_id" binding:"required"` // 会话ID
	Title     string `json:"title" binding:"required"`     // 新标题
}

// CreateChatWithMessageRequest 创建会话并添加首条消息的请求
type CreateChatWithMessageRequest struct {
	Title    string                 `json:"title,omitempty"`            // 会话标题，可选
	Content  string                 `json:"content" binding:"required"` // 消息内容
	Metadata map[string]interface{} `json:"metadata,omitempty"`         // 消息元数据，可选
}

// DeleteChatRequest 删除聊天会话请求
type DeleteChatRequest struct {
	SessionID string `uri:"session_id" binding:"required"` // 会话ID
}

// GetRecentQuestionsRequest 获取最近问题请求
type GetRecentQuestionsRequest struct {
	Limit int `form:"limit,default=10" json:"limit,default=10"` // 返回问题数量限制，默认10条
}

// GetRecentQuestionsResponse 获取最近问题响应
type GetRecentQuestionsResponse struct {
	Questions []string `json:"questions"` // 问题列表
}

// ChatMessageResponse 聊天消息响应对象
type ChatMessageResponse struct {
	ID        uint                   `json:"id"`                 // 消息ID
	SessionID string                 `json:"session_id"`         // 会话ID
	Role      string                 `json:"role"`               // 消息角色
	Content   string                 `json:"content"`            // 消息内容
	CreatedAt time.Time              `json:"created_at"`         // 创建时间
	Sources   []QASourceInfo         `json:"sources,omitempty"`  // 引用来源(如果有)
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // 元数据(如果有)
}

// CreateChatWithMessageResponse 创建会话并添加消息的响应
type CreateChatWithMessageResponse struct {
	SessionID string              `json:"session_id"` // 会话ID
	Title     string              `json:"title"`      // 会话标题
	CreatedAt time.Time           `json:"created_at"` // 创建时间
	Message   ChatMessageResponse `json:"message"`    // 创建的消息
}

// DeleteChatResponse 删除会话响应
type DeleteChatResponse struct {
	Success   bool   `json:"success"`    // 是否成功
	SessionID string `json:"session_id"` // 会话ID
}

// ChatStatistics 聊天统计信息
type ChatStatistics struct {
	TotalSessions int       `json:"total_sessions"`          // 会话总数
	TotalMessages int       `json:"total_messages"`          // 消息总数
	LastActivity  time.Time `json:"last_activity"`           // 最近活动时间
	TopQuestions  []string  `json:"top_questions,omitempty"` // 热门问题(可选)
}
