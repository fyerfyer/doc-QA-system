package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// QA API测试环境配置
type qaTestEnv struct {
	Router          *gin.Engine
	VectorDB        vectordb.Repository
	EmbeddingClient *embedding.MockClient
	LLMClient       *llm.MockClient
	Cache           cache.Cache
	QAService       *services.QAService
}

// 创建QA API测试环境
func setupQATestEnv(t *testing.T) *qaTestEnv {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建内存向量数据库
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:         "memory",
		Dimension:    1536,
		DistanceType: vectordb.Cosine,
	})
	require.NoError(t, err)

	// 创建Mock嵌入客户端
	mockEmbedding := embedding.NewMockClient(t)
	mockEmbedding.On("Name").Maybe().Return("mock-embedding")
	mockEmbedding.On("Embed", mock.Anything, mock.Anything).Maybe().Return(
		make([]float32, 1536), nil, // 返回一个1536维的零向量
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

	// 创建内存缓存
	cacheService, err := cache.NewCache(cache.Config{
		Type:            "memory",
		DefaultTTL:      time.Hour,
		CleanupInterval: time.Minute,
	})
	require.NoError(t, err)

	// 创建RAG服务
	ragService := llm.NewRAG(mockLLM,
		llm.WithRAGMaxTokens(1024),
		llm.WithRAGTemperature(0.7),
	)

	// 创建问答服务
	qaService := services.NewQAService(
		mockEmbedding,
		vectorDB,
		mockLLM,
		ragService,
		cacheService,
		services.WithMinScore(0.0),
	)

	// 创建QA处理器
	qaHandler := handler.NewQAHandler(qaService)

	// 设置路由
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api")
	api.POST("/qa", qaHandler.AnswerQuestion)

	return &qaTestEnv{
		Router:          router,
		VectorDB:        vectorDB,
		EmbeddingClient: mockEmbedding,
		LLMClient:       mockLLM,
		Cache:           cacheService,
		QAService:       qaService,
	}
}

// TestQA 测试问答API
func TestQA(t *testing.T) {
	env := setupQATestEnv(t)

	// 添加一个测试文档到向量数据库
	testDoc := vectordb.Document{
		ID:        "test_doc",
		FileID:    "test_file",
		FileName:  "test.txt",
		Position:  1,
		Text:      "向量数据库是一种专门用于高效存储和检索向量数据的数据库系统。",
		Vector:    make([]float32, 1536), // 使用和 Mock 嵌入维度相同的向量
		CreatedAt: time.Now(),
	}
	env.VectorDB.Add(testDoc)

	// 配置 Mock 嵌入客户端返回一个匹配的向量
	env.EmbeddingClient.On("Embed", mock.Anything, "什么是向量数据库?").Return(
		make([]float32, 1536), nil,
	)

	// 准备问题请求
	reqBody := map[string]interface{}{
		"question": "什么是向量数据库?",
	}
	jsonData, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// 发送问答请求
	req := httptest.NewRequest(http.MethodPost, "/api/qa", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证回答
	qaResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "什么是向量数据库?", qaResp["question"])
	assert.Equal(t, "这是一个模拟回答", qaResp["answer"])
}

// TestQAWithRealAPI 测试使用真实API的问答功能
// 仅当环境变量TONGYI_API_KEY存在时运行
func TestQAWithRealAPI(t *testing.T) {
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("TONGYI_API_KEY environment variable not set, skipping real API test")
	}

	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建真实的通义千问客户端
	tongyiEmbedding, err := embedding.NewClient("tongyi",
		embedding.WithAPIKey(apiKey),
		embedding.WithModel("text-embedding-v1"),
	)
	require.NoError(t, err)

	tongyiLLM, err := llm.NewClient("tongyi",
		llm.WithAPIKey(apiKey),
		llm.WithModel("qwen-turbo"),
	)
	require.NoError(t, err)

	// 创建内存向量数据库
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:         "memory",
		Dimension:    1536,
		DistanceType: vectordb.Cosine,
	})
	require.NoError(t, err)

	// 创建内存缓存
	cacheService, err := cache.NewCache(cache.Config{
		Type: "memory",
	})
	require.NoError(t, err)

	// 创建RAG服务
	ragService := llm.NewRAG(tongyiLLM,
		llm.WithRAGMaxTokens(256),
		llm.WithRAGTemperature(0.7),
	)

	// 创建问答服务
	qaService := services.NewQAService(
		tongyiEmbedding,
		vectorDB,
		tongyiLLM,
		ragService,
		cacheService,
	)

	// 创建问答处理器
	qaHandler := handler.NewQAHandler(qaService)

	// 创建路由器
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api")
	api.POST("/qa", qaHandler.AnswerQuestion)

	// 准备一个简单问题
	reqBody := map[string]interface{}{
		"question": "1+1等于几?",
	}
	jsonData, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// 发送问答请求
	req := httptest.NewRequest(http.MethodPost, "/api/qa", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证有回答
	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	qaResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, qaResp["answer"])
}

// TestQAWithSpecificFile 测试使用特定文件的问答API
func TestQAWithSpecificFile(t *testing.T) {
	env := setupQATestEnv(t)

	// 准备一个测试文档ID
	testFileID := "test_file_123"

	// 将测试文档添加到向量数据库
	testDocument := vectordb.Document{
		ID:        "test_segment_1",
		FileID:    testFileID,
		FileName:  "test.pdf",
		Position:  1,
		Text:      "向量数据库是一种专门存储和检索向量数据的数据库系统",
		Vector:    make([]float32, 1536), // 零向量
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"source": "test",
		},
	}
	err := env.VectorDB.Add(testDocument)
	require.NoError(t, err)

	// 配置Mock行为，用于搜索特定文档
	env.EmbeddingClient.On("Embed", mock.Anything, "什么是向量数据库?").Return(
		make([]float32, 1536), nil,
	)

	// 准备问题请求，指定文件ID
	reqBody := map[string]interface{}{
		"question": "什么是向量数据库?",
		"file_id":  testFileID,
	}
	jsonData, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// 发送问答请求
	req := httptest.NewRequest(http.MethodPost, "/api/qa", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证回答
	qaResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "什么是向量数据库?", qaResp["question"])
	assert.Equal(t, "这是一个模拟回答", qaResp["answer"])
}

// TestHealthCheck 测试健康检查API
func TestHealthCheck(t *testing.T) {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建一个简单的路由来测试健康检查
	router := gin.New()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// 请求健康检查
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}
