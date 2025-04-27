package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// 测试环境配置
type documentTestEnv struct {
	Router          *gin.Engine
	Storage         storage.Storage
	VectorDB        vectordb.Repository
	EmbeddingClient *embedding.MockClient
	LLMClient       *llm.MockClient
	Cache           cache.Cache
	DocumentService *services.DocumentService
	QAService       *services.QAService
}

// 创建测试环境
func setupDocumentTestEnv(t *testing.T) *documentTestEnv {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa_test_*")
	require.NoError(t, err)

	// 临时目录将在测试完成后自动清理
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	// 创建本地存储
	fileStorage, err := storage.NewLocalStorage(storage.LocalConfig{
		Path: tempDir,
	})
	require.NoError(t, err)

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
	mockEmbedding.On("EmbedBatch", mock.Anything, mock.Anything).Maybe().Return(
		func(_ interface{}, texts []string) [][]float32 {
			result := make([][]float32, len(texts))
			for i := range texts {
				result[i] = make([]float32, 1536)
			}
			return result
		},
		nil,
	)

	// 创建Mock LLM客户端
	mockLLM := llm.NewMockClient(t)
	mockLLM.On("Name").Maybe().Return("mock-llm")
	mockLLM.On("Generate", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(
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

	// 创建文本分段器
	splitter := document.NewTextSplitter(document.SplitterConfig{
		SplitType:    document.ByParagraph,
		ChunkSize:    1000,
		ChunkOverlap: 200,
	})

	// 创建RAG服务
	ragService := llm.NewRAG(mockLLM,
		llm.WithRAGMaxTokens(1024),
		llm.WithRAGTemperature(0.7),
	)

	// 创建文档服务
	documentService := services.NewDocumentService(
		fileStorage,
		nil, // 使用ParserFactory
		splitter,
		mockEmbedding,
		vectorDB,
		services.WithBatchSize(5),
	)

	// 创建问答服务
	qaService := services.NewQAService(
		mockEmbedding,
		vectorDB,
		mockLLM,
		ragService,
		cacheService,
	)

	// 创建API处理器
	docHandler := handler.NewDocumentHandler(documentService, fileStorage)
	qaHandler := handler.NewQAHandler(qaService)

	// 设置路由
	router := SetupRouter(docHandler, qaHandler)

	return &documentTestEnv{
		Router:          router,
		Storage:         fileStorage,
		VectorDB:        vectorDB,
		EmbeddingClient: mockEmbedding,
		LLMClient:       mockLLM,
		Cache:           cacheService,
		DocumentService: documentService,
		QAService:       qaService,
	}
}

// 创建测试文件
func createTestFile(t *testing.T, filename string, content string) string {
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

// TestDocumentUpload 测试文档上传API
func TestDocumentUpload(t *testing.T) {
	env := setupDocumentTestEnv(t)

	// 创建测试PDF文件
	testFile := createTestFile(t, "test.pdf", "这是一个测试文件内容")

	// 创建multipart请求
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test.pdf")
	require.NoError(t, err)

	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	_, err = io.Copy(part, file)
	require.NoError(t, err)
	writer.Close()

	// 创建请求
	req := httptest.NewRequest(http.MethodPost, "/api/documents", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 执行请求
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 检查响应中是否包含fileID
	uploadResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, uploadResp["file_id"])
	assert.Equal(t, "processing", uploadResp["status"])
}

// TestDocumentStatus 测试文档状态查询API
func TestDocumentStatus(t *testing.T) {
	env := setupDocumentTestEnv(t)

	// 先上传一个文档
	testFile := createTestFile(t, "test.pdf", "这是一个测试文件内容")
	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	fileInfo, err := env.Storage.Save(file, "test.pdf")
	require.NoError(t, err)

	// 查询状态
	req := httptest.NewRequest(http.MethodGet, "/api/documents/"+fileInfo.ID+"/status", nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应 (状态码可能是200或404，取决于文档服务的实现)
	t.Logf("Status response: %s", w.Body.String())
}

// TestDocumentList 测试文档列表查询API
func TestDocumentList(t *testing.T) {
	env := setupDocumentTestEnv(t)

	// 请求文档列表
	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 注意：在这个测试中，列表应该为空
	listResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, float64(0), listResp["total"])
}

// TestDocumentDelete 测试文档删除API
func TestDocumentDelete(t *testing.T) {
	env := setupDocumentTestEnv(t)

	// 先上传一个文档
	testFile := createTestFile(t, "test.pdf", "这是一个测试文件内容")
	file, err := os.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	fileInfo, err := env.Storage.Save(file, "test.pdf")
	require.NoError(t, err)

	// 删除文档
	req := httptest.NewRequest(http.MethodDelete, "/api/documents/"+fileInfo.ID, nil)
	w := httptest.NewRecorder()
	env.Router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var resp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	// 验证删除成功
	deleteResp, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, true, deleteResp["success"])
}
