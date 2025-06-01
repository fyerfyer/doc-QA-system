package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 集成测试环境结构体
type integrationTestEnv struct {
	DB             *gorm.DB
	ChatRepo       repository.ChatRepository
	DocRepo        repository.DocumentRepository
	ChatService    *services.ChatService
	QAService      *services.QAService
	VectorDB       vectordb.Repository
	EmbedClient    *embedding.MockClient
	LLMClient      *llm.MockClient
	DocService     *services.DocumentService
	StorageService storage.Storage
	Cleanup        func()
}

// 创建集成测试环境
func setupIntegrationTest(t *testing.T) *integrationTestEnv {
	// 创建测试目录
	tempDir, err := os.MkdirTemp("", "qa-chat-integration-test-*")
	require.NoError(t, err, "Failed to create temp directory")

	// 设置数据库
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err, "Failed to create test database")

	// 执行数据迁移
	err = db.AutoMigrate(&models.Document{}, &models.DocumentSegment{},
		&models.ChatSession{}, &models.ChatMessage{})
	require.NoError(t, err, "Failed to run migrations")

	// 保存原始数据库并替换为测试数据库
	originalDB := database.DB
	database.DB = db

	// 创建仓库
	chatRepo := repository.NewChatRepository()
	docRepo := repository.NewDocumentRepository()

	// 创建嵌入客户端（mock）
	embedClient := embedding.NewMockClient(t)
	embedClient.On("Name").Maybe().Return("test-embed")
	embedClient.On("Embed", mock.Anything, mock.Anything).Maybe().Return(
		make([]float32, 4), nil,
	)
	embedClient.On("EmbedBatch", mock.Anything, mock.Anything).Maybe().Return(
		[][]float32{make([]float32, 4)}, nil,
	)

	// 创建向量数据库
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:      "memory",
		Dimension: 4,
	})
	require.NoError(t, err, "Failed to create vector database")

	// 创建LLM客户端（mock）
	llmClient := llm.NewMockClient(t)
	llmClient.On("Name").Maybe().Return("test-llm")
	llmClient.On("Generate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe().Return(
		&llm.Response{
			Text:       "This is a test answer containing important information",
			TokenCount: 5,
			ModelName:  "test-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	llmClient.On("Chat", mock.Anything, mock.Anything).Maybe().Return(
		&llm.Response{
			Text:       "The test document contains important information.",
			Messages:   []llm.Message{{Role: "assistant", Content: "The test document contains important information."}},
			TokenCount: 8,
			ModelName:  "test-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	// 创建RAG服务
	ragService := llm.NewRAG(llmClient)

	// 创建缓存
	memCache, err := cache.NewMemoryCache(cache.DefaultConfig())
	require.NoError(t, err, "Failed to create memory cache")

	// 创建存储服务
	storageService, err := storage.NewLocalStorage(storage.LocalConfig{
		Path: filepath.Join(tempDir, "storage"),
	})
	require.NoError(t, err, "Failed to create storage service")

	// 创建文本分割器
	splitter := document.NewTextSplitter(document.DefaultSplitterConfig())

	// 创建文档状态管理器
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	statusManager := services.NewDocumentStatusManager(docRepo, logger)

	// 创建文档服务
	docService := services.NewDocumentService(
		storageService,
		nil, // 使用解析器工厂
		splitter,
		embedClient,
		vectorDB,
		services.WithStatusManager(statusManager),
	)

	// 创建QA服务
	qaService := services.NewQAService(
		embedClient,
		vectorDB,
		llmClient,
		ragService,
		memCache,
		services.WithMinScore(0.0), // 测试时接受任意分数
	)

	// 创建聊天服务
	chatService := services.NewChatService(chatRepo, services.WithChatLogger(logger))

	// 创建清理函数
	cleanup := func() {
		// 恢复原始数据库
		database.DB = originalDB
		// 删除临时目录
		os.RemoveAll(tempDir)
	}

	return &integrationTestEnv{
		DB:             db,
		ChatRepo:       chatRepo,
		DocRepo:        docRepo,
		ChatService:    chatService,
		QAService:      qaService,
		VectorDB:       vectorDB,
		EmbedClient:    embedClient,
		LLMClient:      llmClient,
		DocService:     docService,
		StorageService: storageService,
		Cleanup:        cleanup,
	}
}

// 测试聊天与QA的基础集成
func TestChatWithQAIntegration(t *testing.T) {
	// 设置测试环境
	env := setupIntegrationTest(t)
	defer env.Cleanup()

	ctx := context.Background()

	// 创建聊天会话
	session, err := env.ChatService.CreateChat(ctx, "Test Chat Session")
	require.NoError(t, err, "Failed to create chat session")
	require.NotNil(t, session, "Chat session should not be nil")

	// 为QA准备——向向量数据库添加测试文档
	testDoc := vectordb.Document{
		ID:        "test-doc-1",
		FileID:    "test-file-1",
		FileName:  "test-document.txt",
		Position:  1,
		Text:      "This is a test document containing important information.",
		Vector:    make([]float32, 4),
		CreatedAt: time.Now(),
	}
	err = env.VectorDB.Add(testDoc)
	require.NoError(t, err, "Failed to add test document to vector database")

	// 向聊天添加用户消息
	userMessage := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   "What is in the test document?",
	}

	// 保存用户消息
	err = env.ChatService.AddMessage(ctx, userMessage)
	require.NoError(t, err, "Failed to add user message")

	// 模拟QA流程
	answer, sources, err := env.QAService.Answer(ctx, userMessage.Content)
	require.NoError(t, err, "QA service should answer without error")
	require.NotEmpty(t, answer, "Answer should not be empty")

	// 构建响应的sources
	var modelSources []models.Source
	for _, src := range sources {
		modelSources = append(modelSources, models.Source{
			FileID:   src.FileID,
			FileName: src.FileName,
			Position: src.Position,
			Text:     src.Text,
		})
	}

	// 添加助手消息
	assistantMessage := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   answer,
	}

	// 保存带sources的消息
	err = env.ChatService.SaveMessageWithSources(ctx, assistantMessage, modelSources)
	require.NoError(t, err, "Failed to add assistant message with sources")

	// 获取聊天历史
	messages, count, err := env.ChatService.GetChatMessages(ctx, session.ID, 0, 10)
	require.NoError(t, err, "Failed to get chat messages")
	require.Equal(t, int64(2), count, "Should have 2 messages")
	require.Len(t, messages, 2, "Should have 2 messages in the result")

	// 检查消息内容
	assert.Equal(t, models.RoleUser, messages[0].Role, "First message should be from user")
	assert.Equal(t, "What is in the test document?", messages[0].Content, "User message content should match")

	assert.Equal(t, models.RoleAssistant, messages[1].Role, "Second message should be from assistant")
	assert.Contains(t, messages[1].Content, "important information", "Assistant message should contain the answer")
}

// 测试QA最近问题与聊天历史的集成
func TestRecentQuestions(t *testing.T) {
	// 设置测试环境
	env := setupIntegrationTest(t)
	defer env.Cleanup()

	ctx := context.Background()

	// 创建多个聊天会话并添加问题
	sessions := make([]*models.ChatSession, 3)
	questions := []string{
		"What is vector database?",
		"How does RAG work?",
		"What is embedding?",
		"Explain LLM architecture",
		"What is prompt engineering?",
	}

	// 创建会话
	for i := 0; i < len(sessions); i++ {
		session, err := env.ChatService.CreateChat(ctx, fmt.Sprintf("Test Session %d", i+1))
		require.NoError(t, err, "Failed to create chat session")
		sessions[i] = session
	}

	// 向不同会话添加问题
	questionDistribution := []struct {
		sessionIndex int
		questionIdx  int
	}{
		{0, 0}, // Session 1, Question 0
		{1, 1}, // Session 2, Question 1
		{2, 2}, // Session 3, Question 2
		{0, 3}, // Session 1, Question 3
		{1, 4}, // Session 2, Question 4
	}

	// 添加每个问题和模拟回复
	for _, qd := range questionDistribution {
		sessionID := sessions[qd.sessionIndex].ID
		question := questions[qd.questionIdx]

		// 添加用户消息
		userMsg := &models.ChatMessage{
			SessionID: sessionID,
			Role:      models.RoleUser,
			Content:   question,
			CreatedAt: time.Now(),
		}
		err := env.ChatService.AddMessage(ctx, userMsg)
		require.NoError(t, err, "Failed to add user message")

		// 添加助手消息
		assistantMsg := &models.ChatMessage{
			SessionID: sessionID,
			Role:      models.RoleAssistant,
			Content:   "This is a test answer for: " + question,
			CreatedAt: time.Now().Add(1 * time.Second),
		}
		err = env.ChatService.AddMessage(ctx, assistantMsg)
		require.NoError(t, err, "Failed to add assistant message")

		// 确保消息时间戳不同
		time.Sleep(10 * time.Millisecond)
	}

	// 测试获取最近问题
	recentMessages, err := env.ChatRepo.GetRecentMessages(10)
	require.NoError(t, err, "Should retrieve recent messages without error")

	// 从消息中提取用户问题
	var extractedQuestions []string
	for _, msg := range recentMessages {
		if msg.Role == models.RoleUser {
			extractedQuestions = append(extractedQuestions, msg.Content)
		}
	}

	// 调用QA服务获取最近问题
	recentQuestions, err := env.QAService.GetRecentQuestions(ctx, 5)
	require.NoError(t, err, "Should get recent questions without error")

	// 验证返回的问题不为空
	require.NotEmpty(t, recentQuestions, "Recent questions should not be empty")

	// 验证问题内容
	for _, question := range recentQuestions {
		assert.Contains(t, questions, question, "Recent question should be in our list of questions")
	}
}

// 测试聊天与已索引文档的交互
func TestChatWithDocumentIntegration(t *testing.T) {
	// 设置测试环境
	env := setupIntegrationTest(t)
	defer env.Cleanup()

	ctx := context.Background()

	// 向向量数据库添加测试文档
	docContents := []struct {
		id       string
		fileID   string
		fileName string
		text     string
	}{
		{
			id:       "doc1",
			fileID:   "file1",
			fileName: "introduction.txt",
			text:     "Vector databases are specialized database systems designed to store and search vectors efficiently.",
		},
		{
			id:       "doc2",
			fileID:   "file1",
			fileName: "introduction.txt",
			text:     "Embeddings are numerical representations of text that capture semantic meaning.",
		},
		{
			id:       "doc3",
			fileID:   "file2",
			fileName: "advanced.txt",
			text:     "RAG systems combine retrieval and generation for improved accuracy in question answering.",
		},
	}

	for _, doc := range docContents {
		err := env.VectorDB.Add(vectordb.Document{
			ID:        doc.id,
			FileID:    doc.fileID,
			FileName:  doc.fileName,
			Text:      doc.text,
			Vector:    make([]float32, 4),
			CreatedAt: time.Now(),
		})
		require.NoError(t, err, "Failed to add document to vector DB")
	}

	// 设置embedding mock
	env.EmbedClient.On("Embed", ctx, "What are vector databases?").Return(
		make([]float32, 4), nil,
	)

	// 创建聊天会话
	session, err := env.ChatService.CreateChat(ctx, "Document Chat Test")
	require.NoError(t, err, "Failed to create chat session")

	// 添加用户问题
	userMsg := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   "What are vector databases?",
	}
	err = env.ChatService.AddMessage(ctx, userMsg)
	require.NoError(t, err, "Failed to add user message")

	// 获取QA服务答案
	answer, sources, err := env.QAService.Answer(ctx, userMsg.Content)
	require.NoError(t, err, "Failed to get answer")

	// 转换sources为模型格式
	var modelSources []models.Source
	for _, src := range sources {
		modelSources = append(modelSources, models.Source{
			FileID:   src.FileID,
			FileName: src.FileName,
			Position: src.Position,
			Text:     src.Text,
		})
	}

	// 添加助手回复
	assistantMsg := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   answer,
	}
	err = env.ChatService.SaveMessageWithSources(ctx, assistantMsg, modelSources)
	require.NoError(t, err, "Failed to save assistant message with sources")

	// 获取聊天消息并验证sources
	messages, _, err := env.ChatService.GetChatMessages(ctx, session.ID, 0, 10)
	require.NoError(t, err, "Failed to retrieve chat messages")
	require.Len(t, messages, 2, "Should have 2 messages")

	// 获取助手消息（第二条）
	assistantMessage := messages[1]
	require.Equal(t, models.RoleAssistant, assistantMessage.Role, "Second message should be from assistant")
	require.NotEmpty(t, assistantMessage.Content, "Assistant message should have content")
}

// 测试带指定文件的问答
func TestAnswerWithSpecificFile(t *testing.T) {
	// 设置测试环境
	env := setupIntegrationTest(t)
	defer env.Cleanup()

	ctx := context.Background()

	// 向向量数据库添加测试文档
	fileID := "specific-file"
	err := env.VectorDB.Add(vectordb.Document{
		ID:        "specific-doc",
		FileID:    fileID,
		FileName:  "specific.txt",
		Text:      "This specific file contains unique information about machine learning.",
		Vector:    make([]float32, 4),
		CreatedAt: time.Now(),
	})
	require.NoError(t, err, "Failed to add document to vector DB")

	env.LLMClient.ExpectedCalls = nil // 清除之前的期望
	env.LLMClient.On("Generate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&llm.Response{
			Text:       "This specific file contains information about machine learning technologies.",
			TokenCount: 10,
			ModelName:  "test-model",
			FinishTime: time.Now(),
		},
		nil,
	)

	// 设置embedding mock
	env.EmbedClient.On("Embed", ctx, "What does the specific file contain?").Return(
		make([]float32, 4), nil,
	)

	// 创建聊天会话
	session, err := env.ChatService.CreateChat(ctx, "Specific File Test")
	require.NoError(t, err, "Failed to create chat session")

	// 添加用户消息
	userMsg := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   "What does the specific file contain?",
	}
	err = env.ChatService.AddMessage(ctx, userMsg)
	require.NoError(t, err, "Failed to add user message")

	// 从指定文件获取QA答案
	answer, sources, err := env.QAService.AnswerWithFile(ctx, userMsg.Content, fileID)
	require.NoError(t, err, "Failed to get answer from specific file")

	// 验证答案和sources
	require.NotEmpty(t, answer, "Answer should not be empty")
	require.NotEmpty(t, sources, "Sources should not be empty")

	// 验证source来自指定文件
	for _, src := range sources {
		assert.Equal(t, fileID, src.FileID, "Source should be from the specific file")
	}

	// 添加助手回复
	assistantMsg := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   answer,
	}

	// 转换sources为模型格式
	var modelSources []models.Source
	for _, src := range sources {
		modelSources = append(modelSources, models.Source{
			FileID:   src.FileID,
			FileName: src.FileName,
			Position: src.Position,
			Text:     src.Text,
		})
	}

	err = env.ChatService.SaveMessageWithSources(ctx, assistantMsg, modelSources)
	require.NoError(t, err, "Failed to save assistant message with sources")

	// 获取聊天消息
	messages, _, err := env.ChatService.GetChatMessages(ctx, session.ID, 0, 10)
	require.NoError(t, err, "Failed to get chat messages")
	require.Len(t, messages, 2, "Should have 2 messages")

	// 验证内容
	assert.Equal(t, "What does the specific file contain?", messages[0].Content, "User question should match")
	assert.Contains(t, messages[1].Content, "machine learning", "Answer should mention machine learning")
}
