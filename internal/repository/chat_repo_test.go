package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupChatTestDB(t *testing.T) (*gorm.DB, func()) {
	// Use in-memory SQLite database for testing
	dbName := fmt.Sprintf("file:memdb_chat_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err, "Failed to open in-memory database")

	// Run migrations
	err = db.AutoMigrate(&models.ChatSession{}, &models.ChatMessage{})
	require.NoError(t, err, "Failed to run migrations")

	// Save original DB reference
	originalDB := database.DB

	// Replace global DB with test DB
	database.DB = db

	// Return cleanup function
	cleanup := func() {
		database.DB = originalDB
	}

	return db, cleanup
}

func TestChatRepository_CreateSession(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create test session
	session := &models.ChatSession{
		ID:        "test-session-1",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Test creation
	err := repo.CreateSession(session)
	assert.NoError(t, err, "Session creation should succeed")

	// Verify session was created
	savedSession, err := repo.GetSession(session.ID)
	assert.NoError(t, err, "Should be able to retrieve created session")
	assert.Equal(t, session.ID, savedSession.ID, "Session ID should match")
	assert.Equal(t, session.Title, savedSession.Title, "Session title should match")
}

func TestChatRepository_GetSession(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Test getting non-existent session
	session, err := repo.GetSession("non-existing")
	assert.Error(t, err, "Should return error for non-existing session")
	assert.Nil(t, session, "Should return nil for non-existing session")

	// Create test session
	testSession := &models.ChatSession{
		ID:        "test-session-2",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = repo.CreateSession(testSession)
	require.NoError(t, err)

	// Test getting existing session
	session, err = repo.GetSession("test-session-2")
	assert.NoError(t, err, "Should retrieve existing session without error")
	assert.NotNil(t, session, "Should return session object")
	assert.Equal(t, "Test Session", session.Title, "Session title should match")
}

func TestChatRepository_ListSessions(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create test sessions
	sessions := []*models.ChatSession{
		{
			ID:        "test-session-3",
			Title:     "Session 1",
			Tags:      "important,first",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID:        "test-session-4",
			Title:     "Session 2",
			Tags:      "important",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:        "test-session-5",
			Title:     "Session 3",
			Tags:      "archive",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	for _, session := range sessions {
		err := repo.CreateSession(session)
		require.NoError(t, err)
	}

	// Test without filters
	resultSessions, total, err := repo.ListSessions(0, 10, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total, "Total count should be 3")
	assert.Len(t, resultSessions, 3, "Should return 3 sessions")

	// Test pagination
	resultSessions, total, err = repo.ListSessions(1, 2, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total, "Total count should still be 3")
	assert.Len(t, resultSessions, 2, "Should return 2 sessions with offset 1")

	// Test tag filtering
	filters := map[string]interface{}{
		"tags": "important",
	}
	resultSessions, total, err = repo.ListSessions(0, 10, filters)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total, "Total count should be 2")
	assert.Len(t, resultSessions, 2, "Should return 2 sessions with important tag")
}

func TestChatRepository_UpdateSession(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create test session
	session := &models.ChatSession{
		ID:        "test-session-6",
		Title:     "Original Title",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repo.CreateSession(session)
	require.NoError(t, err, "Session creation should succeed")

	// Update session
	session.Title = "Updated Title"
	session.Tags = "important,updated"

	err = repo.UpdateSession(session)
	assert.NoError(t, err, "Session update should succeed")

	// Verify update
	updatedSession, err := repo.GetSession(session.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Title", updatedSession.Title, "Title should be updated")
	assert.Equal(t, "important,updated", updatedSession.Tags, "Tags should be updated")
	assert.True(t, updatedSession.UpdatedAt.After(session.CreatedAt), "Updated time should be after creation time")
}

func TestChatRepository_DeleteSession(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// 创建测试会话
	session := &models.ChatSession{
		ID:        "test-session-7",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repo.CreateSession(session)
	require.NoError(t, err)

	message := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   "Test message",
		CreatedAt: time.Now(),
	}
	err = repo.CreateMessage(message)
	require.NoError(t, err)

	// 验证删除
	err = repo.DeleteSession(session.ID)
	assert.NoError(t, err, "Delete should succeed")

	// 验证会话已删除
	_, err = repo.GetSession(session.ID)
	assert.Error(t, err, "Session should no longer exist")

	// 验证消息也被删除
	messages, _, err := repo.GetMessages(session.ID, 0, 10)
	assert.Error(t, err, "GetMessages should error on deleted session")
	assert.Contains(t, err.Error(), "chat session not found")
	assert.Empty(t, messages, "Messages should be empty")
}

func TestChatRepository_CreateMessage(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create test session
	session := &models.ChatSession{
		ID:        "test-session-8",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := repo.CreateSession(session)
	require.NoError(t, err)

	// Create message
	message := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   "Hello, world!",
		CreatedAt: time.Now(),
	}

	err = repo.CreateMessage(message)
	assert.NoError(t, err, "Message creation should succeed")
	assert.Greater(t, message.ID, uint(0), "Message should have an ID assigned")

	// Check session updated time
	updatedSession, err := repo.GetSession(session.ID)
	assert.NoError(t, err)
	assert.True(t, updatedSession.UpdatedAt.After(session.UpdatedAt) ||
		updatedSession.UpdatedAt.Equal(session.UpdatedAt),
		"Session updated time should be updated")
}

func TestChatRepository_GetMessages(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create test session
	session := &models.ChatSession{
		ID:        "test-session-9",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := repo.CreateSession(session)
	require.NoError(t, err)

	// Add multiple messages
	for i := 0; i < 5; i++ {
		message := &models.ChatMessage{
			SessionID: session.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Message %d", i+1),
			CreatedAt: time.Now(),
		}
		err = repo.CreateMessage(message)
		require.NoError(t, err)
	}

	// Test getting all messages
	messages, count, err := repo.GetMessages(session.ID, 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count, "Total message count should be 5")
	assert.Len(t, messages, 5, "Should return 5 messages")

	// Test pagination
	messages, count, err = repo.GetMessages(session.ID, 2, 2)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count, "Total message count should be 5")
	assert.Len(t, messages, 2, "Should return 2 messages with offset 2")
}

func TestChatRepository_GetRecentMessages(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create two test sessions
	session1 := &models.ChatSession{
		ID:        "test-session-10",
		Title:     "Session 1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	session2 := &models.ChatSession{
		ID:        "test-session-11",
		Title:     "Session 2",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repo.CreateSession(session1)
	require.NoError(t, err)
	err = repo.CreateSession(session2)
	require.NoError(t, err)

	// Add messages with time delays to ensure ordering
	for i := 0; i < 3; i++ {
		message := &models.ChatMessage{
			SessionID: session1.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Session 1 Message %d", i+1),
			CreatedAt: time.Now().Add(time.Duration(-3+i) * time.Second),
		}
		err = repo.CreateMessage(message)
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		message := &models.ChatMessage{
			SessionID: session2.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Session 2 Message %d", i+1),
			CreatedAt: time.Now(),
		}
		err = repo.CreateMessage(message)
		require.NoError(t, err)
	}

	// Test getting recent messages
	messages, err := repo.GetRecentMessages(4)
	assert.NoError(t, err)
	assert.Len(t, messages, 4, "Should return 4 recent messages")

	// The most recent messages should be from session 2
	assert.Equal(t, session2.ID, messages[0].SessionID, "Most recent message should be from session 2")
}

func TestChatRepository_CountMessages(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create test session
	session := &models.ChatSession{
		ID:        "test-session-12",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := repo.CreateSession(session)
	require.NoError(t, err)

	// Add multiple messages
	for i := 0; i < 3; i++ {
		message := &models.ChatMessage{
			SessionID: session.ID,
			Role:      models.RoleUser,
			Content:   fmt.Sprintf("Message %d", i+1),
			CreatedAt: time.Now(),
		}
		err = repo.CreateMessage(message)
		require.NoError(t, err)
	}

	// Test counting messages
	count, err := repo.CountMessages(session.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count, "Message count should be 3")
}

func TestChatRepository_WithContext(t *testing.T) {
	_, cleanup := setupChatTestDB(t)
	defer cleanup()

	repo := NewChatRepository()

	// Create context
	ctx := context.Background()

	// Use WithContext to create a new repo with context
	repoWithCtx := repo.WithContext(ctx)

	// Ensure the new repo is not nil
	assert.NotNil(t, repoWithCtx, "Repository with context should not be nil")

	// Test that the repo with context works
	session := &models.ChatSession{
		ID:        "test-session-13",
		Title:     "Test With Context",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := repoWithCtx.CreateSession(session)
	assert.NoError(t, err, "Creating session with context should succeed")

	// Verify can retrieve the session
	retrievedSession, err := repoWithCtx.GetSession(session.ID)
	assert.NoError(t, err)
	assert.Equal(t, session.ID, retrievedSession.ID)
}
