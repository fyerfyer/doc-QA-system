package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChatRepository 聊天仓储接口
// 负责聊天会话和消息的存储和检索
type ChatRepository interface {
	// CreateSession 创建聊天会话
	CreateSession(session *models.ChatSession) error

	// GetSession 获取聊天会话
	GetSession(id string) (*models.ChatSession, error)

	// ListSessions 列出聊天会话，支持分页和筛选
	ListSessions(offset, limit int, filters map[string]interface{}) ([]*models.ChatSession, int64, error)

	// UpdateSession 更新聊天会话
	UpdateSession(session *models.ChatSession) error

	// DeleteSession 删除聊天会话
	DeleteSession(id string) error

	// CreateMessage 创建聊天消息
	CreateMessage(message *models.ChatMessage) error

	// GetMessages 获取会话消息列表
	GetMessages(sessionID string, offset, limit int) ([]*models.ChatMessage, int64, error)

	// GetRecentMessages 获取最近的消息
	GetRecentMessages(limit int) ([]*models.ChatMessage, error)

	// CountMessages 统计会话消息数量
	CountMessages(sessionID string) (int64, error)

	// WithContext 创建带有上下文的仓储
	WithContext(ctx context.Context) ChatRepository
}

// chatRepo 聊天仓储实现
type chatRepo struct {
	db *gorm.DB // 数据库连接
}

// NewChatRepository 创建聊天仓储实例
func NewChatRepository() ChatRepository {
	return &chatRepo{
		db: database.MustDB(),
	}
}

// NewChatRepositoryWithDB 使用指定的数据库连接创建聊天仓储实例
func NewChatRepositoryWithDB(db *gorm.DB) ChatRepository {
	if db == nil {
		db = database.MustDB()
	}
	return &chatRepo{
		db: db,
	}
}

// WithContext 创建带有上下文的仓储
func (r *chatRepo) WithContext(ctx context.Context) ChatRepository {
	return &chatRepo{
		db: r.db.WithContext(ctx),
	}
}

// CreateSession 创建聊天会话
func (r *chatRepo) CreateSession(session *models.ChatSession) error {
	if session.ID == "" {
		session.ID = uuid.New().String()
	}

	// 确保时间字段被设置
	now := time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now

	return r.db.Create(session).Error
}

// GetSession 获取聊天会话
func (r *chatRepo) GetSession(id string) (*models.ChatSession, error) {
	var session models.ChatSession
	err := r.db.Where("id = ?", id).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("chat session not found: %s", id)
		}
		return nil, err
	}
	return &session, nil
}

// ListSessions 列出聊天会话，支持分页和筛选
func (r *chatRepo) ListSessions(offset, limit int, filters map[string]interface{}) ([]*models.ChatSession, int64, error) {
	var sessions []*models.ChatSession
	var total int64

	// 创建查询构造器
	query := r.db.Model(&models.ChatSession{})

	// 应用筛选条件
	if filters != nil {
		// 用户ID过滤
		if userID, ok := filters["user_id"].(string); ok && userID != "" {
			query = query.Where("user_id = ?", userID)
		}

		// 标签过滤
		if tags, ok := filters["tags"].(string); ok && tags != "" {
			// 使用LIKE查询匹配包含指定标签的会话
			query = query.Where("tags LIKE ?", "%"+tags+"%")
		}

		// 时间范围过滤
		if startTime, ok := filters["start_time"].(time.Time); ok {
			query = query.Where("created_at >= ?", startTime)
		}

		if endTime, ok := filters["end_time"].(time.Time); ok {
			query = query.Where("created_at <= ?", endTime)
		}

		// 标题关键词搜索
		if title, ok := filters["title"].(string); ok && title != "" {
			query = query.Where("title LIKE ?", "%"+title+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 应用排序和分页
	err = query.Order("updated_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&sessions).Error

	if err != nil {
		return nil, 0, err
	}

	return sessions, total, nil
}

// UpdateSession 更新聊天会话
func (r *chatRepo) UpdateSession(session *models.ChatSession) error {
	if session.ID == "" {
		return errors.New("session ID cannot be empty")
	}

	// 确保更新时间被更新
	session.UpdatedAt = time.Now()

	return r.db.Save(session).Error
}

// DeleteSession 删除聊天会话
func (r *chatRepo) DeleteSession(id string) error {
	// 开启事务
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 删除会话的所有消息
		if err := tx.Where("session_id = ?", id).Delete(&models.ChatMessage{}).Error; err != nil {
			return err
		}

		// 2. 删除会话记录
		if err := tx.Where("id = ?", id).Delete(&models.ChatSession{}).Error; err != nil {
			return err
		}

		return nil
	})
}

// CreateMessage 创建聊天消息
func (r *chatRepo) CreateMessage(message *models.ChatMessage) error {
	if message.SessionID == "" {
		return errors.New("session ID cannot be empty")
	}

	// 确保时间字段被设置
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	// 创建消息记录
	if err := r.db.Create(message).Error; err != nil {
		return err
	}

	// 更新会话的最后更新时间
	return r.db.Model(&models.ChatSession{}).
		Where("id = ?", message.SessionID).
		Update("updated_at", time.Now()).Error
}

// GetMessages 获取会话消息列表
func (r *chatRepo) GetMessages(sessionID string, offset, limit int) ([]*models.ChatMessage, int64, error) {
	var messages []*models.ChatMessage
	var total int64

	// 先检查会话是否存在
	var exists int64
	err := r.db.Model(&models.ChatSession{}).
		Where("id = ?", sessionID).
		Count(&exists).Error

	if err != nil {
		return nil, 0, err
	}

	if exists == 0 {
		return nil, 0, fmt.Errorf("chat session not found: %s", sessionID)
	}

	// 获取消息总数
	err = r.db.Model(&models.ChatMessage{}).
		Where("session_id = ?", sessionID).
		Count(&total).Error

	if err != nil {
		return nil, 0, err
	}

	// 查询消息列表
	err = r.db.Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Offset(offset).
		Limit(limit).
		Find(&messages).Error

	if err != nil {
		return nil, 0, err
	}

	return messages, total, nil
}

// GetRecentMessages 获取最近的消息
func (r *chatRepo) GetRecentMessages(limit int) ([]*models.ChatMessage, error) {
	var messages []*models.ChatMessage

	err := r.db.Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error

	if err != nil {
		return nil, err
	}

	return messages, nil
}

// CountMessages 统计会话消息数量
func (r *chatRepo) CountMessages(sessionID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.ChatMessage{}).
		Where("session_id = ?", sessionID).
		Count(&count).Error

	return count, err
}
