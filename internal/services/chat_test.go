package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupChatTestEnv(t *testing.T) (*ChatService, func()) {
	// 创建一个内存中的 SQLite 数据库用于测试
	dbName := fmt.Sprintf("file:memdb_chat_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err, "Failed to open in-memory database")

	// 运行数据库迁移
	err = db.AutoMigrate(&models.ChatSession{}, &models.ChatMessage{})
	require.NoError(t, err, "Failed to run migrations")

	// 保存原始数据库引用
	originalDB := database.DB

	// 替换全局数据库为测试数据库
	database.DB = db

	// 创建日志记录器
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// 创建仓库和服务
	chatRepo := repository.NewChatRepository()
	chatService := NewChatService(chatRepo, WithChatLogger(logger))

	// 返回清理函数
	cleanup := func() {
		database.DB = originalDB
	}

	return chatService, cleanup
}

func TestChatService_CreateChat(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 测试自定义标题
	title := "Test Chat Session"
	session, err := chatService.CreateChat(ctx, title)
	require.NoError(t, err)
	assert.Equal(t, title, session.Title)
	assert.NotEmpty(t, session.ID)
	assert.False(t, session.CreatedAt.IsZero())
	assert.False(t, session.UpdatedAt.IsZero())

	// 测试空标题（应使用默认值）
	session, err = chatService.CreateChat(ctx, "")
	require.NoError(t, err)
	assert.NotEmpty(t, session.Title)
	assert.NotEmpty(t, session.ID)
	assert.Contains(t, session.Title, "新对话")
}

func TestChatService_GetChatSession(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建一个测试会话
	title := "Test Get Session"
	createdSession, err := chatService.CreateChat(ctx, title)
	require.NoError(t, err)

	// 测试获取会话
	session, err := chatService.GetChatSession(ctx, createdSession.ID)
	assert.NoError(t, err)
	assert.Equal(t, createdSession.ID, session.ID)
	assert.Equal(t, title, session.Title)

	// 测试获取不存在的会话
	_, err = chatService.GetChatSession(ctx, "non-existing-id")
	assert.Error(t, err, "Should return error for non-existing session")
}

func TestChatService_ListChatSessions(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建多个测试会话
	titles := []string{"Session 1", "Session 2", "Session 3"}
	for _, title := range titles {
		_, err := chatService.CreateChat(ctx, title)
		require.NoError(t, err)
	}

	// 测试列出会话（无过滤器）
	sessions, total, err := chatService.ListChatSessions(ctx, 0, 10, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, sessions, 3)

	// 测试分页
	sessions, total, err = chatService.ListChatSessions(ctx, 1, 2, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, sessions, 2)

	// 测试带过滤器
	_, err = chatService.CreateChat(ctx, "Test With Tags")
	require.NoError(t, err)

	// 更新会话以添加标签
	session, err := chatService.GetChatSession(ctx, sessions[0].ID)
	require.NoError(t, err)
	session.Tags = "important,test"
	err = chatService.UpdateChatSession(ctx, session)
	require.NoError(t, err)

	// 按标签过滤
	filters := map[string]interface{}{
		"tags": "important",
	}
	filteredSessions, filteredTotal, err := chatService.ListChatSessions(ctx, 0, 10, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), filteredTotal)
	assert.Len(t, filteredSessions, 1)
	assert.Contains(t, filteredSessions[0].Tags, "important")
}

func TestChatService_UpdateAndDeleteChatSession(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建一个测试会话
	originalTitle := "Original Title"
	session, err := chatService.CreateChat(ctx, originalTitle)
	require.NoError(t, err)

	// 更新会话
	session.Title = "Updated Title"
	session.Tags = "important,test"
	err = chatService.UpdateChatSession(ctx, session)
	assert.NoError(t, err)

	// 验证更新
	updatedSession, err := chatService.GetChatSession(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Title", updatedSession.Title)
	assert.Equal(t, "important,test", updatedSession.Tags)

	// 删除会话
	err = chatService.DeleteChatSession(ctx, session.ID)
	assert.NoError(t, err)

	// 验证删除
	_, err = chatService.GetChatSession(ctx, session.ID)
	assert.Error(t, err, "Session should no longer exist")
}

func TestChatService_AddAndGetMessages(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建一个测试会话
	session, err := chatService.CreateChat(ctx, "Test Messages")
	require.NoError(t, err)

	// 添加消息
	messages := []*models.ChatMessage{
		{
			SessionID: session.ID,
			Role:      models.RoleUser,
			Content:   "Hello!",
			CreatedAt: time.Now(),
		},
		{
			SessionID: session.ID,
			Role:      models.RoleAssistant,
			Content:   "Hi there!",
			CreatedAt: time.Now().Add(time.Second),
		},
		{
			SessionID: session.ID,
			Role:      models.RoleSystem,
			Content:   "System message",
			CreatedAt: time.Now().Add(2 * time.Second),
		},
	}

	for _, msg := range messages {
		err = chatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 获取消息
	retrievedMessages, count, err := chatService.GetChatMessages(ctx, session.ID, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.Len(t, retrievedMessages, 3)

	// 检查消息顺序（应按创建时间排序）
	assert.Equal(t, models.RoleUser, retrievedMessages[0].Role)
	assert.Equal(t, models.RoleAssistant, retrievedMessages[1].Role)
	assert.Equal(t, models.RoleSystem, retrievedMessages[2].Role)

	// 测试分页
	retrievedMessages, count, err = chatService.GetChatMessages(ctx, session.ID, 1, 2)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.Len(t, retrievedMessages, 2)
}

func TestChatService_CountMessages(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建一个测试会话
	session, err := chatService.CreateChat(ctx, "Test Count")
	require.NoError(t, err)

	// 添加消息
	for i := 0; i < 5; i++ {
		msg := &models.ChatMessage{
			SessionID: session.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Message %d", i+1),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = chatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 统计消息
	count, err := chatService.CountChatMessages(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestChatService_GetRecentMessages(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建测试会话
	session1, err := chatService.CreateChat(ctx, "Session 1")
	require.NoError(t, err)

	session2, err := chatService.CreateChat(ctx, "Session 2")
	require.NoError(t, err)

	// 向会话 1 添加消息
	for i := 0; i < 3; i++ {
		msg := &models.ChatMessage{
			SessionID: session1.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Session 1 Message %d", i+1),
			CreatedAt: time.Now().Add(-time.Duration(5-i) * time.Second),
		}
		err = chatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 向会话 2 添加消息（更近期）
	for i := 0; i < 3; i++ {
		msg := &models.ChatMessage{
			SessionID: session2.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Session 2 Message %d", i+1),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = chatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 获取最近的消息
	recentMessages, err := chatService.GetRecentMessages(ctx, 4)
	assert.NoError(t, err)
	assert.Len(t, recentMessages, 4, "Should return 4 most recent messages")

	// 最近的消息应来自会话 2
	assert.Equal(t, session2.ID, recentMessages[0].SessionID, "Most recent message should be from session 2")
}

func TestChatService_SaveMessageWithSources(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建一个测试会话
	session, err := chatService.CreateChat(ctx, "Test Sources")
	require.NoError(t, err)

	// 创建带来源的消息
	message := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   "This is a response with sources",
	}

	sources := []models.Source{
		{
			FileID:   "file-1",
			FileName: "document1.pdf",
			Position: 1,
			Text:     "Source text from document 1",
			Score:    0.95,
		},
		{
			FileID:   "file-2",
			FileName: "document2.pdf",
			Position: 2,
			Text:     "Source text from document 2",
			Score:    0.85,
		},
	}

	// 保存带来源的消息
	err = chatService.SaveMessageWithSources(ctx, message, sources)
	assert.NoError(t, err)
	assert.Greater(t, message.ID, uint(0), "Message should have an ID assigned")

	// 检索消息
	messages, _, err := chatService.GetChatMessages(ctx, session.ID, 0, 10)
	assert.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.NotEmpty(t, messages[0].Sources, "Sources should be saved")
}

func TestChatService_RenameChatSession(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建一个测试会话
	oldTitle := "Old Title"
	session, err := chatService.CreateChat(ctx, oldTitle)
	require.NoError(t, err)

	// 重命名会话
	newTitle := "New Title"
	err = chatService.RenameChatSession(ctx, session.ID, newTitle)
	assert.NoError(t, err)

	// 验证标题已更改
	updatedSession, err := chatService.GetChatSession(ctx, session.ID)
	assert.NoError(t, err)
	assert.Equal(t, newTitle, updatedSession.Title, "Session title should be updated")
}

func TestChatService_GetChatsWithMessageCount(t *testing.T) {
	chatService, cleanup := setupChatTestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建测试会话
	session1, err := chatService.CreateChat(ctx, "Session with 3 messages")
	require.NoError(t, err)

	session2, err := chatService.CreateChat(ctx, "Session with 1 message")
	require.NoError(t, err)

	// 向会话 1 添加消息
	for i := 0; i < 3; i++ {
		msg := &models.ChatMessage{
			SessionID: session1.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Message %d", i+1),
		}
		err = chatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 向会话 2 添加消息
	msg := &models.ChatMessage{
		SessionID: session2.ID,
		Role:      models.RoleUser,
		Content:   "Single message",
	}
	err = chatService.AddMessage(ctx, msg)
	require.NoError(t, err)

	// 获取带消息计数的会话
	chats, total, err := chatService.GetChatsWithMessageCount(ctx, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, chats, 2)

	// 在结果中找到每个会话并验证消息计数
	for _, chat := range chats {
		sessionID, ok := chat["id"].(string)
		require.True(t, ok, "Session ID should be a string")

		messageCount, ok := chat["message_count"].(int64)
		require.True(t, ok, "Message count should be present")

		if sessionID == session1.ID {
			assert.Equal(t, int64(3), messageCount, "Session 1 should have 3 messages")
		} else if sessionID == session2.ID {
			assert.Equal(t, int64(1), messageCount, "Session 2 should have 1 message")
		}
	}
}
