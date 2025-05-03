package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// MessageRole 消息角色类型
type MessageRole string

const (
	// RoleUser 用户角色
	RoleUser MessageRole = "user"
	// RoleSystem 系统角色
	RoleSystem MessageRole = "system"
	// RoleAssistant 助手角色
	RoleAssistant MessageRole = "assistant"
)

// ChatSession 聊天会话模型
// 用于存储用户的聊天会话信息
type ChatSession struct {
	ID        string         `gorm:"primaryKey"`        // 会话ID，主键
	Title     string         `gorm:"not null"`          // 会话标题
	CreatedAt time.Time      `gorm:"not null"`          // 创建时间
	UpdatedAt time.Time      `gorm:"not null"`          // 更新时间
	UserID    string         `gorm:"index"`             // 用户标识，可选
	Tags      string         `gorm:"type:varchar(255)"` // 标签，逗号分隔
	Metadata  datatypes.JSON `gorm:"type:json"`         // 元数据，JSON格式
}

// BeforeCreate GORM的钩子函数，创建记录前自动设置时间
func (cs *ChatSession) BeforeCreate(tx *gorm.DB) (err error) {
	now := time.Now()
	if cs.CreatedAt.IsZero() {
		cs.CreatedAt = now
	}
	cs.UpdatedAt = now
	return nil
}

// BeforeUpdate GORM的钩子函数，更新记录前自动设置更新时间
func (cs *ChatSession) BeforeUpdate(tx *gorm.DB) (err error) {
	cs.UpdatedAt = time.Now()
	return nil
}

// TableName 明确指定表名
func (ChatSession) TableName() string {
	return "chat_sessions"
}

// ChatMessage 聊天消息模型
// 用于存储会话中的单条消息
type ChatMessage struct {
	ID        uint           `gorm:"primaryKey;autoIncrement"`  // 主键ID
	SessionID string         `gorm:"not null;index"`            // 所属会话ID
	Role      MessageRole    `gorm:"not null;type:varchar(20)"` // 消息角色
	Content   string         `gorm:"type:text;not null"`        // 消息内容
	CreatedAt time.Time      `gorm:"not null"`                  // 创建时间
	Metadata  datatypes.JSON `gorm:"type:json"`                 // 元数据
	Sources   datatypes.JSON `gorm:"type:json"`                 // 引用的信息源
}

// BeforeCreate GORM的钩子函数，创建记录前自动设置时间
func (cm *ChatMessage) BeforeCreate(tx *gorm.DB) (err error) {
	if cm.CreatedAt.IsZero() {
		cm.CreatedAt = time.Now()
	}
	return nil
}

// TableName 明确指定表名
func (ChatMessage) TableName() string {
	return "chat_messages"
}

// Source 表示消息引用的信息源
type Source struct {
	FileID   string  `json:"file_id"`         // 文件ID
	FileName string  `json:"file_name"`       // 文件名
	Position int     `json:"position"`        // 段落位置
	Text     string  `json:"text"`            // 引用的文本
	Score    float32 `json:"score,omitempty"` // 匹配分数
}
