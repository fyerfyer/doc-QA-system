package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// chatTestEnv 测试环境配置
type chatTestEnv struct {
	Router        *gin.Engine
	DB            *gorm.DB
	ChatRepo      repository.ChatRepository
	QAService     *services.QAService
	ChatService   *services.ChatService
	ChatHandler   *handler.ChatHandler
	MockLLM       *llm.MockClient
	MockEmbedding *embedding.MockClient
}

// setupChatTestEnv 创建Chat测试环境
func setupChatTestEnv(t *testing.T) *chatTestEnv {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建内存数据库
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err, "Failed to create in-memory database")

	// 运行数据库迁移
	err = db.AutoMigrate(&models.ChatSession{}, &models.ChatMessage{})
	require.NoError(t, err, "Failed to run migrations")

	// 保存原始数据库引用并替换为测试数据库
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})

	// 创建Mock嵌入客户端
	mockEmbedding := embedding.NewMockClient(t)
	mockEmbedding.On("Name").Maybe().Return("mock-embedding")
	mockEmbedding.On("Embed", mock.Anything, mock.Anything).Maybe().Return(
		make([]float32, 1536), nil,
	)

	// 创建Mock LLM客户端
	mockLLM := llm.NewMockClient(t)
	mockLLM.On("Name").Maybe().Return("mock-llm")
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe().Return(
		&llm.Response{
			Text:       "这是一个模拟回答",
			TokenCount: 10,
			ModelName:  "mock-model",
			FinishTime: time.Now(),
		},
		nil,
	)
	mockLLM.On("Chat", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(
		&llm.Response{
			Text:       "这是一个模拟回答",
			Messages:   []llm.Message{{Role: llm.RoleAssistant, Content: "这是一个模拟回答"}},
			TokenCount: 10,
			ModelName:  "mock-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	// 创建内存向量数据库
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:      "memory",
		Dimension: 1536,
	})
	require.NoError(t, err, "Failed to create vector database")

	// 创建内存缓存
	memoryCache, err := cache.NewMemoryCache(cache.DefaultConfig())
	require.NoError(t, err, "Failed to create memory cache")

	// 创建RAG服务
	ragService := llm.NewRAG(mockLLM)

	// 创建QA服务
	qaService := services.NewQAService(
		mockEmbedding,
		vectorDB,
		mockLLM,
		ragService,
		memoryCache,
	)

	// 创建聊天仓储和服务
	chatRepo := repository.NewChatRepository()
	chatService := services.NewChatService(chatRepo)

	// 创建聊天处理器
	chatHandler := handler.NewChatHandler(chatService, qaService)

	// 设置路由
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api")
	// 聊天API路由
	chatGroup := api.Group("/chats")
	chatGroup.POST("", chatHandler.CreateChat)
	chatGroup.GET("", chatHandler.ListChats)
	chatGroup.POST("/with-message", chatHandler.CreateChatWithMessage)
	chatGroup.POST("/messages", chatHandler.AddMessage)
	chatGroup.GET("/:session_id", chatHandler.GetChatHistory)
	chatGroup.PATCH("/:session_id", chatHandler.RenameChat)
	chatGroup.DELETE("/:session_id", chatHandler.DeleteChat)
	api.GET("/recent-questions", chatHandler.GetRecentQuestions)

	return &chatTestEnv{
		Router:        router,
		DB:            db,
		ChatRepo:      chatRepo,
		QAService:     qaService,
		ChatService:   chatService,
		ChatHandler:   chatHandler,
		MockLLM:       mockLLM,
		MockEmbedding: mockEmbedding,
	}
}

// TestCreateChat 测试创建聊天会话
func TestCreateChat(t *testing.T) {
	env := setupChatTestEnv(t)

	// 准备请求数据
	reqData := map[string]interface{}{
		"title": "测试聊天会话",
	}
	jsonData, err := json.Marshal(reqData)
	require.NoError(t, err)

	// 发送创建聊天请求
	req := httptest.NewRequest("POST", "/api/chats", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证返回的会话数据
	chat, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, chat["chat_id"])
	assert.Equal(t, "测试聊天会话", chat["title"])
	assert.NotEmpty(t, chat["created_at"])
}

// TestListChats 测试列出聊天会话
func TestListChats(t *testing.T) {
	env := setupChatTestEnv(t)

	// 创建几个测试会话
	// 使用 gin.CreateTestContext 创建上下文
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	for i := 1; i <= 3; i++ {
		session, err := env.ChatService.CreateChat(ctx, "测试会话")
		require.NoError(t, err)

		// 为每个会话添加一条消息
		msg := &models.ChatMessage{
			SessionID: session.ID,
			Role:      models.RoleUser,
			Content:   "测试消息",
		}
		err = env.ChatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 发送列表请求
	req := httptest.NewRequest("GET", "/api/chats?page=1&page_size=10", nil)
	w = httptest.NewRecorder() // 重新创建一个 recorder
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证返回的会话列表
	listResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, float64(3), listResp["total"])
	assert.Equal(t, float64(1), listResp["page"])
	assert.Equal(t, float64(10), listResp["page_size"])

	chats, ok := listResp["chats"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, chats, 3)
}

// TestGetChatHistory 测试获取聊天历史
func TestGetChatHistory(t *testing.T) {
	env := setupChatTestEnv(t)

	// 创建测试会话
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	session, err := env.ChatService.CreateChat(ctx, "测试历史会话")
	require.NoError(t, err)

	// 添加几条测试消息
	messages := []struct {
		role    models.MessageRole
		content string
	}{
		{models.RoleUser, "你好，这是用户消息"},
		{models.RoleAssistant, "你好，这是助手回复"},
		{models.RoleUser, "再次问候"},
		{models.RoleAssistant, "再次回复"},
	}

	for _, m := range messages {
		msg := &models.ChatMessage{
			SessionID: session.ID,
			Role:      m.role,
			Content:   m.content,
		}
		err = env.ChatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 发送获取历史请求
	req := httptest.NewRequest("GET", "/api/chats/"+session.ID, nil)
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证返回的历史消息
	history, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, session.ID, history["chat_id"])
	assert.Equal(t, "测试历史会话", history["title"])

	chatMessages, ok := history["messages"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, chatMessages, 4)
}

// TestAddMessage 测试添加消息
func TestAddMessage(t *testing.T) {
	env := setupChatTestEnv(t)

	// 创建测试会话
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	session, err := env.ChatService.CreateChat(ctx, "测试添加消息")
	require.NoError(t, err)

	// 准备请求数据
	reqData := map[string]interface{}{
		"session_id": session.ID,
		"role":       "user",
		"content":    "这是一个测试消息",
	}
	jsonData, err := json.Marshal(reqData)
	require.NoError(t, err)

	// 设置应答生成预期
	env.MockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&llm.Response{
			Text:       "这是自动生成的回复",
			TokenCount: 10,
			ModelName:  "mock-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	// 发送添加消息请求
	req := httptest.NewRequest("POST", "/api/chats/messages", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证消息已添加
	messages, count, err := env.ChatService.GetChatMessages(ctx, session.ID, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count) // 用户消息和自动生成的助手回复
	assert.Equal(t, "这是一个测试消息", messages[0].Content)
	assert.Equal(t, models.RoleUser, messages[0].Role)
}

// TestRenameChat 测试重命名聊天会话
func TestRenameChat(t *testing.T) {
	env := setupChatTestEnv(t)

	// 创建测试会话
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	session, err := env.ChatService.CreateChat(ctx, "原始标题")
	require.NoError(t, err)

	// 准备请求数据
	reqData := map[string]interface{}{
		"title": "新标题",
	}
	jsonData, err := json.Marshal(reqData)
	require.NoError(t, err)

	// 发送重命名请求
	req := httptest.NewRequest("PATCH", "/api/chats/"+session.ID, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	env.Router.ServeHTTP(w, req)

	// 验证响应
	fmt.Printf("DEBUG: Response status code: %d\n", w.Code)
	fmt.Printf("DEBUG: Response body: %s\n", w.Body.String())

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证标题已更新
	updatedSession, err := env.ChatService.GetChatSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "新标题", updatedSession.Title)
}

// TestDeleteChat 测试删除聊天会话
func TestDeleteChat(t *testing.T) {
	env := setupChatTestEnv(t)

	// 创建测试会话
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	session, err := env.ChatService.CreateChat(ctx, "待删除会话")
	require.NoError(t, err)

	// 发送删除请求
	req := httptest.NewRequest("DELETE", "/api/chats/"+session.ID, nil)
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证会话已删除
	_, err = env.ChatService.GetChatSession(ctx, session.ID)
	assert.Error(t, err, "Should return error when session not found")
}

// TestCreateChatWithMessage 测试创建会话并添加消息
func TestCreateChatWithMessage(t *testing.T) {
	env := setupChatTestEnv(t)

	// 尝试清除已有的期望
	env.MockLLM.ExpectedCalls = nil
	env.MockLLM.On("Generate",
		mock.Anything, // ctx
		mock.Anything, // prompt
		mock.Anything, // option1
		mock.Anything, // option2,
	).Return(
		&llm.Response{
			Text:       "这是对问题的回答",
			TokenCount: 10,
			ModelName:  "mock-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	// 准备请求数据
	reqData := map[string]interface{}{
		"title":   "测试聊天",
		"content": "这是第一条消息",
	}
	jsonData, err := json.Marshal(reqData)
	require.NoError(t, err)

	// 发送创建请求
	req := httptest.NewRequest("POST", "/api/chats/with-message", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证返回数据
	result, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, result["session_id"])
	assert.Equal(t, "测试聊天", result["title"])

	message, ok := result["message"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "assistant", message["role"])
	assert.NotEmpty(t, message["content"])
}

// TestGetRecentQuestions 测试获取最近问题
func TestGetRecentQuestions(t *testing.T) {
	env := setupChatTestEnv(t)

	// 创建测试会话和消息
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	session, err := env.ChatService.CreateChat(ctx, "测试会话")
	require.NoError(t, err)

	// 添加一些测试消息
	messages := []struct {
		role    models.MessageRole
		content string
	}{
		{models.RoleUser, "问题1"},
		{models.RoleAssistant, "回答1"},
		{models.RoleUser, "问题2"},
		{models.RoleAssistant, "回答2"},
		{models.RoleUser, "问题3"},
		{models.RoleAssistant, "回答3"},
	}

	for _, m := range messages {
		msg := &models.ChatMessage{
			SessionID: session.ID,
			Role:      m.role,
			Content:   m.content,
		}
		err = env.ChatService.AddMessage(ctx, msg)
		require.NoError(t, err)
	}

	// 发送请求
	req := httptest.NewRequest("GET", "/api/recent-questions?limit=5", nil)
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证返回的问题列表
	result, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)

	questions, ok := result["questions"].([]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, questions)
}

// TestInvalidRequests 测试无效请求处理
func TestInvalidRequests(t *testing.T) {
	env := setupChatTestEnv(t)

	// 测试添加消息但未提供必要字段
	reqData := map[string]interface{}{
		"role": "user",
		// 缺少session_id和content
	}
	jsonData, _ := json.Marshal(reqData)

	req := httptest.NewRequest("POST", "/api/chats/messages", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 应该返回错误
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 测试获取不存在的聊天历史
	req = httptest.NewRequest("GET", "/api/chats/non-existent-id", nil)
	w = httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 应该返回错误
	assert.NotEqual(t, http.StatusOK, w.Code)
}
