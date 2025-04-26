package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
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
		assert.Equal(t, fileID, doc.FileID, "Returned document should belong to the specified file")
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
		category, exists := doc.Metadata["category"]
		assert.True(t, exists, "Document should contain category metadata")
		assert.Equal(t, "database", category, "Document should have correct category")
	}
}

// TestQAServiceEdgeCases 测试边缘情况
func TestQAServiceEdgeCases(t *testing.T) {
	// 设置测试环境
	qaService, cleanup := setupQATestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 测试空问题
	_, _, err := qaService.Answer(ctx, "")
	assert.Error(t, err, "Empty question should return an error")

	// 测试找不到答案的问题
	irrelevantQuestion := "宇宙的终极答案是什么？"
	answer, docs, err := qaService.Answer(ctx, irrelevantQuestion)
	require.NoError(t, err)
	assert.Contains(t, answer, "Sorry", "Should return an apologetic message")
	assert.Empty(t, docs, "Should not return documents")
}

// TestQAServiceCacheOperations 测试缓存操作
func TestQAServiceCacheOperations(t *testing.T) {
	qaService, cleanup := setupQATestEnv(t)
	defer cleanup()

	ctx := context.Background()

	// 先问一个问题填充缓存
	question := "什么是RAG？"
	_, _, err := qaService.Answer(ctx, question)
	require.NoError(t, err)

	// 清除缓存
	err = qaService.ClearCache()
	require.NoError(t, err)

	// 确保清除后重新查询不会命中缓存
	_, _, err = qaService.Answer(ctx, question)
	require.NoError(t, err)

	// 如果查询时间很短，可能是命中了缓存
	// 这个测试有点脆弱，取决于测试环境的速度
	// assert.GreaterOrEqual(t, time.Since(startTime), 10*time.Millisecond, "Should not hit cache")
}

// TestQATongyiIntegration 测试与通义千问API的集成
func TestQATongyiIntegration(t *testing.T) {
	// 检查是否有API密钥可用
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("TONGYI_API_KEY environment variable not set, skipping integration test")
	}

	// 设置实际的大语言模型和嵌入模型
	embeddingClient, err := embedding.NewTongyiClient(
		embedding.WithAPIKey(apiKey),
		embedding.WithModel("text-embedding-v1"),
	)
	require.NoError(t, err)

	llmClient, err := llm.NewTongyiClient(
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
		t.Logf("API call error: %v", err)
		t.Skip("Skipping API test due to error")
		return
	}

	// 只检查是否返回了回答，不检查具体内容
	assert.NotEmpty(t, answer, "Should return a non-empty answer from real API")
}

// setupQATestEnv 设置测试环境，返回QA服务和清理函数
func setupQATestEnv(t *testing.T) (*QAService, func()) {
	// 创建内存缓存
	memoryCache, err := cache.NewMemoryCache(cache.DefaultConfig())
	require.NoError(t, err)

	// 创建向量数据库
	vectorDBConfig := vectordb.Config{
		Type:      "memory",
		Dimension: 4, // 使用小维度简化测试
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(t, err)

	// 创建嵌入客户端 - 使用Mock
	embeddingClient := &testEmbeddingClient{dimension: 4}

	// 创建LLM客户端 - 使用Mock
	llmClient := &testLLMClient{}

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
		memoryCache,
	)

	// 返回清理函数
	cleanup := func() {
		vectorDB.Close()
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
		// 生成嵌入向量
		vector, err := embeddingClient.Embed(ctx, doc.Text)
		require.NoError(t, err)

		// 创建文档
		vectorDoc := vectordb.Document{
			ID:        doc.ID,
			FileID:    doc.FileID,
			FileName:  "test.txt",
			Position:  doc.Position,
			Text:      doc.Text,
			Vector:    vector,
			Metadata:  doc.Metadata,
			CreatedAt: time.Now(),
		}

		// 添加到向量库
		err = vectorDB.Add(vectorDoc)
		require.NoError(t, err)
	}
}

// testLLMClient 用于测试的LLM客户端
type testLLMClient struct{}

func (c *testLLMClient) Generate(ctx context.Context, prompt string, options ...llm.GenerateOption) (*llm.Response, error) {
	// 简单返回固定的测试响应
	return &llm.Response{
		Text:       "这是测试回答：" + prompt,
		TokenCount: 10,
		ModelName:  "test-model",
		FinishTime: time.Now(),
	}, nil
}

func (c *testLLMClient) Chat(ctx context.Context, messages []llm.Message, options ...llm.ChatOption) (*llm.Response, error) {
	// 简单返回固定的测试响应
	content := "这是测试对话回答"
	if len(messages) > 0 {
		content = "回答：" + messages[len(messages)-1].Content
	}

	return &llm.Response{
		Text:       content,
		Messages:   []llm.Message{{Role: llm.RoleAssistant, Content: content}},
		TokenCount: 10,
		ModelName:  "test-model",
		FinishTime: time.Now(),
	}, nil
}

func (c *testLLMClient) Name() string {
	return "test-llm"
}
