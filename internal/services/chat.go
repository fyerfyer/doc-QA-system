package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ChatService 聊天服务
// 负责管理聊天会话和消息的业务逻辑
type ChatService struct {
	repo   repository.ChatRepository // 聊天仓储接口
	logger *logrus.Logger            // 日志记录器
}

// ChatOption 聊天服务配置选项
type ChatOption func(*ChatService)

// NewChatService 创建聊天服务实例
func NewChatService(repo repository.ChatRepository, opts ...ChatOption) *ChatService {
	// 创建服务实例
	service := &ChatService{
		repo:   repo,
		logger: logrus.New(),
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// WithChatLogger 设置日志记录器
func WithChatLogger(logger *logrus.Logger) ChatOption {
	return func(s *ChatService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithChatRepository 设置聊天仓储接口
func WithChatRepository(repo repository.ChatRepository) ChatOption {
	return func(s *ChatService) {
		s.repo = repo
	}
}

// CreateChat 创建新的聊天会话
func (s *ChatService) CreateChat(ctx context.Context, title string) (*models.ChatSession, error) {
	if title == "" {
		title = "新对话 " + time.Now().Format("2006-01-02 15:04:05")
	}

	// 创建会话对象
	session := &models.ChatSession{
		ID:        uuid.New().String(),
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 保存到数据库
	err := s.repo.CreateSession(session)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create chat session")
		return nil, fmt.Errorf("failed to create chat session: %w", err)
	}

	s.logger.WithField("session_id", session.ID).Info("Chat session created")
	return session, nil
}

// GetChatSession 获取聊天会话详情
func (s *ChatService) GetChatSession(ctx context.Context, sessionID string) (*models.ChatSession, error) {
	if sessionID == "" {
		return nil, errors.New("session ID cannot be empty")
	}

	// 从仓储获取会话
	session, err := s.repo.GetSession(sessionID)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to get chat session")
		return nil, fmt.Errorf("failed to get chat session: %w", err)
	}

	return session, nil
}

// ListChatSessions 列出聊天会话
func (s *ChatService) ListChatSessions(ctx context.Context, offset, limit int, filters map[string]interface{}) ([]*models.ChatSession, int64, error) {
	// 从仓储获取会话列表
	sessions, total, err := s.repo.ListSessions(offset, limit, filters)
	if err != nil {
		s.logger.WithError(err).Error("Failed to list chat sessions")
		return nil, 0, fmt.Errorf("failed to list chat sessions: %w", err)
	}

	return sessions, total, nil
}

// UpdateChatSession 更新聊天会话
func (s *ChatService) UpdateChatSession(ctx context.Context, session *models.ChatSession) error {
	if session.ID == "" {
		return errors.New("session ID cannot be empty")
	}

	// 确保更新时间被设置
	session.UpdatedAt = time.Now()

	// 保存到数据库
	err := s.repo.UpdateSession(session)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", session.ID).Error("Failed to update chat session")
		return fmt.Errorf("failed to update chat session: %w", err)
	}

	s.logger.WithField("session_id", session.ID).Info("Chat session updated")
	return nil
}

// DeleteChatSession 删除聊天会话
func (s *ChatService) DeleteChatSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("session ID cannot be empty")
	}

	// 从数据库删除
	err := s.repo.DeleteSession(sessionID)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to delete chat session")
		return fmt.Errorf("failed to delete chat session: %w", err)
	}

	s.logger.WithField("session_id", sessionID).Info("Chat session deleted")
	return nil
}

// AddMessage 添加聊天消息
func (s *ChatService) AddMessage(ctx context.Context, message *models.ChatMessage) error {
	if message.SessionID == "" {
		return errors.New("session ID cannot be empty")
	}

	if message.Content == "" {
		return errors.New("message content cannot be empty")
	}

	// 确保消息角色有效
	if message.Role != models.RoleUser &&
		message.Role != models.RoleSystem &&
		message.Role != models.RoleAssistant {
		message.Role = models.RoleUser // 默认为用户角色
	}

	// 确保创建时间被设置
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	// 保存到数据库
	err := s.repo.CreateMessage(message)
	if err != nil {
		s.logger.WithError(err).
			WithFields(logrus.Fields{
				"session_id": message.SessionID,
				"role":       message.Role,
			}).Error("Failed to add chat message")
		return fmt.Errorf("failed to add chat message: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"session_id": message.SessionID,
		"role":       message.Role,
	}).Info("Chat message added")
	return nil
}

// GetChatMessages 获取会话消息列表
func (s *ChatService) GetChatMessages(ctx context.Context, sessionID string, offset, limit int) ([]*models.ChatMessage, int64, error) {
	if sessionID == "" {
		return nil, 0, errors.New("session ID cannot be empty")
	}

	// 从仓储获取消息
	messages, total, err := s.repo.GetMessages(sessionID, offset, limit)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to get chat messages")
		return nil, 0, fmt.Errorf("failed to get chat messages: %w", err)
	}

	return messages, total, nil
}

// GetRecentMessages 获取最近的消息
func (s *ChatService) GetRecentMessages(ctx context.Context, limit int) ([]*models.ChatMessage, error) {
	if limit <= 0 {
		limit = 10 // 默认获取10条
	}

	// 从仓储获取最近消息
	messages, err := s.repo.GetRecentMessages(limit)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get recent messages")
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}

	return messages, nil
}

// CountChatMessages 统计会话消息数量
func (s *ChatService) CountChatMessages(ctx context.Context, sessionID string) (int64, error) {
	if sessionID == "" {
		return 0, errors.New("session ID cannot be empty")
	}

	// 统计消息数量
	count, err := s.repo.CountMessages(sessionID)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to count chat messages")
		return 0, fmt.Errorf("failed to count chat messages: %w", err)
	}

	return count, nil
}

// SaveMessageWithSources 保存带有引用来源的消息
func (s *ChatService) SaveMessageWithSources(ctx context.Context, message *models.ChatMessage, sources []models.Source) error {
	if message.SessionID == "" {
		return errors.New("session ID cannot be empty")
	}

	if message.Content == "" {
		return errors.New("message content cannot be empty")
	}

	// 将来源信息序列化为JSON
	if len(sources) > 0 {
		sourcesJSON, err := json.Marshal(sources)
		if err != nil {
			s.logger.WithError(err).Error("Failed to marshal sources to JSON")
			return fmt.Errorf("failed to marshal sources: %w", err)
		}

		// 将JSON赋值给消息的Sources字段
		message.Sources = sourcesJSON
	}

	// 保存到数据库
	err := s.repo.CreateMessage(message)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", message.SessionID).Error("Failed to save message with sources")
		return fmt.Errorf("failed to save message with sources: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"session_id":    message.SessionID,
		"sources_count": len(sources),
	}).Info("Message with sources saved")
	return nil
}

// RenameChatSession 重命名聊天会话
func (s *ChatService) RenameChatSession(ctx context.Context, sessionID string, newTitle string) error {
	if sessionID == "" {
		return errors.New("session ID cannot be empty")
	}

	if newTitle == "" {
		return errors.New("new title cannot be empty")
	}

	// 获取会话
	session, err := s.repo.GetSession(sessionID)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to get chat session for rename")
		return fmt.Errorf("failed to get chat session: %w", err)
	}

	// 更新标题
	session.Title = newTitle
	session.UpdatedAt = time.Now()

	// 保存更新
	err = s.repo.UpdateSession(session)
	if err != nil {
		s.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to rename chat session")
		return fmt.Errorf("failed to rename chat session: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"new_title":  newTitle,
	}).Info("Chat session renamed")
	return nil
}

// GetChatsWithMessageCount 获取带消息数量的聊天会话列表
func (s *ChatService) GetChatsWithMessageCount(ctx context.Context, offset, limit int) ([]map[string]interface{}, int64, error) {
	// 获取会话列表
	sessions, total, err := s.repo.ListSessions(offset, limit, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list chat sessions: %w", err)
	}

	// 准备返回结果
	result := make([]map[string]interface{}, len(sessions))

	// 为每个会话添加消息数量
	for i, session := range sessions {
		// 获取消息数量
		count, err := s.repo.CountMessages(session.ID)
		if err != nil {
			s.logger.WithError(err).WithField("session_id", session.ID).Warn("Failed to count messages")
			count = 0 // 出错时默认为0
		}

		// 构建带有消息数量的会话信息
		result[i] = map[string]interface{}{
			"id":            session.ID,
			"title":         session.Title,
			"created_at":    session.CreatedAt,
			"updated_at":    session.UpdatedAt,
			"message_count": count,
		}
	}

	return result, total, nil
}
