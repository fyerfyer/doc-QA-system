package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestQAService 测试问答服务的基本功能
func TestQAService(t *testing.T) {
	// 设置测试环境
	qaService, cleanup := setupQATestEnv(t)
	defer cleanup()

	// 测试基本问答功能
	ctx := context.Background()
	question := "什么是向量数据库？"
	answer, docs, err := qaService.Answer(ctx, question)
	require.NoError(t, err)
	assert.NotEmpty(t, answer, "Should return a non-empty answer")
	assert.NotEmpty(t, docs, "Should return source documents")

	// 测试缓存功能
	cachedAnswer, cachedDocs, err := qaService.Answer(ctx, question)
	require.NoError(t, err)
	assert.Equal(t, answer, cachedAnswer, "Cached answer should match")
	assert.Equal(t, len(docs), len(cachedDocs), "Cached document count should match")
}

// TestQAServiceWithFile 测试针对特定文件的问答
func TestQAServiceWithFile(t *testing.T) {
	// 设置测试环境
	qaService, cleanup := setupQATestEnv(t)
	defer cleanup()

	// 测试特定文件问答
	ctx := context.Background()
	fileID := "test-file-1" // 与setupQATestEnv中创建的文件ID保持一致
	question := "向量数据库的优点是什么？"

	answer, docs, err := qaService.AnswerWithFile(ctx, question, fileID)
	require.NoError(t, err)
	assert.NotEmpty(t, answer, "Should return a non-empty answer")

	// 检查返回的文档是否属于指定文件
	for _, doc := range docs {
		assert.Equal(t, fileID, doc.FileID, "Document should be from the specified file")
	}
}

// TestQAServiceWithMetadata 测试带元数据过滤的问答
func TestQAServiceWithMetadata(t *testing.T) {
	// 设置测试环境
	qaService, cleanup := setupQATestEnv(t)
	defer cleanup()

	// 测试元数据过滤问答
	ctx := context.Background()
	metadata := map[string]interface{}{
		"category": "database",
	}
	question := "向量数据库有哪些？"

	answer, docs, err := qaService.AnswerWithMetadata(ctx, question, metadata)
	require.NoError(t, err)
	assert.NotEmpty(t, answer, "Should return a non-empty answer")

	// 检查返回的文档是否包含指定元数据
	for _, doc := range docs {
		category, ok := doc.Metadata["category"]
		assert.True(t, ok, "Document should have category metadata")
		assert.Equal(t, "database", category, "Document should have correct category")
	}
}

// TestQAServiceCacheOperations 测试缓存操作
func TestQAServiceCacheOperations(t *testing.T) {
	// 设置测试环境，使用内存缓存
	memoryCache, err := cache.NewMemoryCache(cache.DefaultConfig())
	require.NoError(t, err)

	qaService, cleanup := setupQATestEnvWithCache(t, memoryCache)
	defer cleanup()

	ctx := context.Background()

	// 生成一个唯一的问题，避免与其他测试干扰
	question := "缓存测试: 什么是RAG？" + time.Now().Format(time.RFC3339Nano)

	// 第一次问题应该不命中缓存
	startTime := time.Now()
	firstAnswer, _, err := qaService.Answer(ctx, question)
	firstQueryTime := time.Since(startTime)
	require.NoError(t, err)
	assert.NotEmpty(t, firstAnswer)

	// 第二次问题应该命中缓存，速度更快
	startTime = time.Now()
	secondAnswer, _, err := qaService.Answer(ctx, question)
	secondQueryTime := time.Since(startTime)
	require.NoError(t, err)
	assert.Equal(t, firstAnswer, secondAnswer, "Cached answer should be the same")

	// 这个断言在某些环境下可能不稳定，但在大多数情况下缓存查询应该显著更快
	t.Logf("First query took %v, second (cached) query took %v", firstQueryTime, secondQueryTime)

	// 清除缓存
	err = qaService.ClearCache()
	require.NoError(t, err)

	// 清除后的查询应该不命中缓存
	startTime = time.Now()
	thirdAnswer, _, err := qaService.Answer(ctx, question)
	thirdQueryTime := time.Since(startTime)
	require.NoError(t, err)
	assert.Equal(t, firstAnswer, thirdAnswer, "Answer content should be consistent")

	// 同样，这个断言在某些环境下可能不稳定
	t.Logf("Second query took %v, third query (after cache clear) took %v", secondQueryTime, thirdQueryTime)
}

// TestQAGetRecentQuestions 测试获取最近问题功能
func TestQAGetRecentQuestions(t *testing.T) {
	// 创建一个临时数据库用于测试
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// 迁移表结构
	err = db.AutoMigrate(&models.ChatSession{}, &models.ChatMessage{})
	require.NoError(t, err)

	// 保存原始数据库并替换为测试数据库
	originalDB := database.DB
	database.DB = db
	defer func() {
		database.DB = originalDB
	}()

	// 创建所需的组件
	chatRepo := repository.NewChatRepository()
	qaService, cleanup := setupQATestEnv(t)
	logger := logrus.New()
	defer cleanup()

	ctx := context.Background()

	// 创建聊天会话
	session, err := (&ChatService{repo: chatRepo, logger: logger}).CreateChat(ctx, "测试会话")
	require.NoError(t, err)

	// 添加一些历史消息
	messages := []struct {
		role    models.MessageRole
		content string
	}{
		{models.RoleUser, "什么是向量数据库？"},
		{models.RoleAssistant, "向量数据库是一种专门用于存储和索引向量数据的数据库。"},
		{models.RoleUser, "RAG技术是什么？"},
		{models.RoleAssistant, "RAG是Retrieval-Augmented Generation的缩写，是一种结合检索和生成的AI技术。"},
		{models.RoleUser, "大语言模型有哪些？"},
		{models.RoleAssistant, "目前流行的大语言模型包括GPT系列、Claude系列和LLaMA系列等。"},
	}

	// 将消息添加到数据库
	for _, msg := range messages {
		err := chatRepo.CreateMessage(&models.ChatMessage{
			SessionID: session.ID,
			Role:      msg.role,
			Content:   msg.content,
			CreatedAt: time.Now(),
		})
		require.NoError(t, err)
		// 添加一点时间间隔，以确保消息的创建时间不同
		time.Sleep(10 * time.Millisecond)
	}

	// 测试获取最近问题
	recentQuestions, err := qaService.GetRecentQuestions(ctx, 5)
	require.NoError(t, err)

	// 验证返回的问题数量
	assert.Len(t, recentQuestions, 3, "Should return 3 user questions")

	// 验证问题内容
	expectedQuestions := []string{
		"大语言模型有哪些？",
		"RAG技术是什么？",
		"什么是向量数据库？",
	}

	// 检查每个预期问题是否存在（顺序可能会因时间戳而不同）
	for _, expected := range expectedQuestions {
		found := false
		for _, actual := range recentQuestions {
			if actual == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected question not found: "+expected)
	}
}

// TestChatHistoryIntegration 测试QA系统与聊天历史的集成
func TestChatHistoryIntegration(t *testing.T) {
	// 创建一个临时数据库用于测试
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// 迁移表结构
	err = db.AutoMigrate(&models.ChatSession{}, &models.ChatMessage{})
	require.NoError(t, err)

	// 保存原始数据库并替换为测试数据库
	originalDB := database.DB
	database.DB = db
	defer func() {
		database.DB = originalDB
	}()

	// 创建所需的组件
	chatRepo := repository.NewChatRepository()
	chatService := NewChatService(chatRepo)
	qaService, cleanup := setupQATestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 创建聊天会话
	session, err := chatService.CreateChat(ctx, "集成测试会话")
	require.NoError(t, err)

	// 用户提问
	userQuestion := "什么是向量数据库？"
	userMsg := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   userQuestion,
	}

	// 添加用户消息
	err = chatService.AddMessage(ctx, userMsg)
	require.NoError(t, err)

	// 使用QA服务生成回答
	answer, sources, err := qaService.Answer(ctx, userQuestion)
	require.NoError(t, err)

	// 转换引用来源
	var modelSources []models.Source
	for _, src := range sources {
		modelSources = append(modelSources, models.Source{
			FileID:   src.FileID,
			FileName: src.FileName,
			Position: src.Position,
			Text:     src.Text,
			Score:    0.9, // 模拟分数
		})
	}

	// 添加助手回复
	assistantMsg := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   answer,
	}

	// 保存回复和来源
	err = chatService.SaveMessageWithSources(ctx, assistantMsg, modelSources)
	require.NoError(t, err)

	// 验证消息已正确保存
	messages, count, err := chatService.GetChatMessages(ctx, session.ID, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Should have 2 messages (user + assistant)")
	assert.Len(t, messages, 2, "Should have retrieved 2 messages")

	// 验证用户问题
	assert.Equal(t, models.RoleUser, messages[0].Role, "First message should be from user")
	assert.Equal(t, userQuestion, messages[0].Content, "User message content should match")

	// 验证助手回复
	assert.Equal(t, models.RoleAssistant, messages[1].Role, "Second message should be from assistant")
	assert.Equal(t, answer, messages[1].Content, "Assistant message content should match")
}

// TestQATongyiIntegration 测试与通义千问API的集成
func TestQATongyiIntegration(t *testing.T) {
	// 检查是否有API密钥可用
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("TONGYI_API_KEY environment variable not set, skipping real API test")
	}

	// 设置实际的大语言模型和嵌入模型
	embeddingClient, err := embedding.NewClient("tongyi",
		embedding.WithAPIKey(apiKey),
		embedding.WithModel("text-embedding-v1"),
	)
	require.NoError(t, err)

	llmClient, err := llm.NewClient("tongyi",
		llm.WithAPIKey(apiKey),
		llm.WithModel(llm.ModelQwenTurbo),
	)
	require.NoError(t, err)

	// 设置其他组件
	vectorDBConfig := vectordb.Config{
		Type:      "memory",
		Dimension: 1536, // 通义嵌入默认维度
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(t, err)

	// 创建测试文档
	createTestDocuments(t, embeddingClient, vectorDB)

	// 创建RAG服务
	ragService := llm.NewRAG(llmClient)

	// 创建缓存
	memoryCache, err := cache.NewMemoryCache(cache.DefaultConfig())
	require.NoError(t, err)

	// 创建问答服务
	qaService := NewQAService(
		embeddingClient,
		vectorDB,
		llmClient,
		ragService,
		memoryCache,
	)

	// 测试简单问题
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	question := "什么是向量数据库？"
	answer, _, err := qaService.Answer(ctx, question)
	if err != nil {
		t.Logf("API error: %v", err)
		t.Skip("Skipping test due to API error")
	}

	// 只检查是否返回了回答，不检查具体内容
	assert.NotEmpty(t, answer, "Should return a non-empty answer from real API")
}

// setupQATestEnv 设置测试环境，返回QA服务和清理函数
func setupQATestEnv(t *testing.T) (*QAService, func()) {
	// 创建内存缓存
	memoryCache, err := cache.NewMemoryCache(cache.DefaultConfig())
	require.NoError(t, err)

	return setupQATestEnvWithCache(t, memoryCache)
}

// setupQATestEnvWithCache 使用指定缓存设置测试环境
func setupQATestEnvWithCache(t *testing.T, cacheInstance cache.Cache) (*QAService, func()) {
	// 创建向量数据库
	vectorDBConfig := vectordb.Config{
		Type:      "memory",
		Dimension: 4, // 使用小维度简化测试
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(t, err)

	// 创建嵌入客户端 - 使用Mock
	embeddingClient := embedding.NewMockClient(t)
	embeddingClient.On("Name").Maybe().Return("mock-embedding")
	embeddingClient.On("Embed", mock.Anything, mock.Anything).Maybe().Return(
		make([]float32, 4), nil,
	)
	embeddingClient.On("EmbedBatch", mock.Anything, mock.Anything).Maybe().Return(
		[][]float32{make([]float32, 4)}, nil,
	)

	// 创建LLM客户端 - 使用Mock
	llmClient := llm.NewMockClient(t)
	llmClient.On("Name").Maybe().Return("mock-llm")
	llmClient.On("Generate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe().Return(
		&llm.Response{
			Text:       "这是测试回答",
			TokenCount: 10,
			ModelName:  "mock-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	// 创建RAG服务
	ragService := llm.NewRAG(llmClient)

	// 创建测试数据
	createTestDocuments(t, embeddingClient, vectorDB)

	// 创建问答服务
	qaService := NewQAService(
		embeddingClient,
		vectorDB,
		llmClient,
		ragService,
		cacheInstance,
		WithMinScore(0.0),
	)

	// 返回清理函数
	cleanup := func() {
	}

	return qaService, cleanup
}

// createTestDocuments 创建测试文档
func createTestDocuments(t *testing.T, embeddingClient embedding.Client, vectorDB vectordb.Repository) {
	ctx := context.Background()

	// 创建测试文档
	docs := []struct {
		ID       string
		FileID   string
		Position int
		Text     string
		Metadata map[string]interface{}
	}{
		{
			ID:       "doc1",
			FileID:   "test-file-1",
			Position: 0,
			Text:     "向量数据库是一种专门用于存储和检索向量数据的数据库系统。",
			Metadata: map[string]interface{}{"category": "database"},
		},
		{
			ID:       "doc2",
			FileID:   "test-file-1",
			Position: 1,
			Text:     "向量数据库的主要优点是能够进行高效的相似度搜索。",
			Metadata: map[string]interface{}{"category": "database"},
		},
		{
			ID:       "doc3",
			FileID:   "test-file-2",
			Position: 0,
			Text:     "RAG（Retrieval-Augmented Generation）是一种结合检索和生成的AI技术。",
			Metadata: map[string]interface{}{"category": "ai"},
		},
	}

	// 添加到向量数据库
	for _, doc := range docs {
		// 获取文本嵌入
		vector, err := embeddingClient.Embed(ctx, doc.Text)
		require.NoError(t, err)

		// 创建向量文档
		vectorDoc := vectordb.Document{
			ID:       doc.ID,
			FileID:   doc.FileID,
			FileName: "test.txt",
			Position: doc.Position,
			Text:     doc.Text,
			Vector:   vector,
			Metadata: doc.Metadata,
		}

		// 添加到向量数据库
		err = vectorDB.Add(vectorDoc)
		require.NoError(t, err)
	}
}
