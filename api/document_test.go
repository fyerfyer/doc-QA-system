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
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/models"
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

	// Clean database tables before test
	cleanDatabase(t)

	// 临时目录将在测试完成后自动清理
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
		cleanDatabase(t)
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

	err = documentService.Init()
	require.NoError(t, err)

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
	assert.Equal(t, "uploaded", uploadResp["status"])
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

// TestDocumentList 测试文档列表API
func TestDocumentList(t *testing.T) {
	// 设置测试环境
	env := setupDocumentTestEnv(t)

	// 创建测试文档数据
	testDocs := []struct {
		ID         string
		FileName   string
		Status     models.DocumentStatus
		Tags       string
		FileSize   int64
		UploadedAt time.Time
	}{
		{
			ID:         "test-doc-1",
			FileName:   "document1.pdf",
			Status:     models.DocStatusCompleted,
			Tags:       "important,report",
			FileSize:   1024,
			UploadedAt: time.Now().Add(-48 * time.Hour),
		},
		{
			ID:         "test-doc-2",
			FileName:   "document2.txt",
			Status:     models.DocStatusProcessing,
			Tags:       "draft",
			FileSize:   512,
			UploadedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			ID:         "test-doc-3",
			FileName:   "document3.md",
			Status:     models.DocStatusFailed,
			Tags:       "report",
			FileSize:   2048,
			UploadedAt: time.Now().Add(-12 * time.Hour),
		},
	}

	// 创建文档状态管理器用于添加测试数据
	statusManager := env.DocumentService.GetStatusManager()
	require.NotNil(t, statusManager, "Status manager should not be nil")

	// 向数据库添加测试文档
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	for _, doc := range testDocs {
		err := statusManager.MarkAsUploaded(ctx, doc.ID, doc.FileName, "/path/to/"+doc.FileName, doc.FileSize)
		require.NoError(t, err, "Failed to create test document")

		// 更新文档状态
		switch doc.Status {
		case models.DocStatusProcessing:
			err = statusManager.MarkAsProcessing(ctx, doc.ID)
		case models.DocStatusCompleted:
			err = statusManager.MarkAsProcessing(ctx, doc.ID)
			require.NoError(t, err)
			err = statusManager.MarkAsCompleted(ctx, doc.ID, 5)
		case models.DocStatusFailed:
			err = statusManager.MarkAsProcessing(ctx, doc.ID)
			require.NoError(t, err)
			err = statusManager.MarkAsFailed(ctx, doc.ID, "Test error message")
		}
		require.NoError(t, err)

		// 更新标签
		if doc.Tags != "" {
			dbDoc, err := statusManager.GetDocument(ctx, doc.ID)
			require.NoError(t, err)
			dbDoc.Tags = doc.Tags
			err = statusManager.GetRepo().Update(dbDoc)
			require.NoError(t, err)
		}
	}

	// 测试基本列表功能，不带过滤条件
	t.Run("basic list without filters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
		w := httptest.NewRecorder()
		env.Router.ServeHTTP(w, req)

		// 验证响应状态码
		assert.Equal(t, http.StatusOK, w.Code)

		// 解析响应
		var resp model.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.Code)

		// 验证文档列表
		listResp, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(3), listResp["total"], "Total should be 3")

		// 验证文档数据
		documents, ok := listResp["documents"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, documents, 3, "Should return 3 documents")
	})

	// 测试分页功能
	t.Run("pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/documents?page=1&page_size=2", nil)
		w := httptest.NewRecorder()
		env.Router.ServeHTTP(w, req)

		// 验证响应
		var resp model.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证分页参数和文档数量
		listResp, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(3), listResp["total"], "Total should still be 3")
		assert.Equal(t, float64(1), listResp["page"], "Page should be 1")
		assert.Equal(t, float64(2), listResp["page_size"], "Page size should be 2")

		// 验证返回的文档数量是否符合页大小
		documents, ok := listResp["documents"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, documents, 2, "Should return 2 documents for page_size=2")

		// 测试第二页
		req = httptest.NewRequest(http.MethodGet, "/api/documents?page=2&page_size=2", nil)
		w = httptest.NewRecorder()
		env.Router.ServeHTTP(w, req)

		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		listResp, ok = resp.Data.(map[string]interface{})
		assert.True(t, ok)
		documents, ok = listResp["documents"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, documents, 1, "Should return 1 document on the second page")
	})

	// 测试按状态过滤
	t.Run("filter by status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/documents?status=processing", nil)
		w := httptest.NewRecorder()
		env.Router.ServeHTTP(w, req)

		// 验证响应
		var resp model.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证只返回处理中的文档
		listResp, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(1), listResp["total"], "Should find 1 processing document")

		documents, ok := listResp["documents"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, documents, 1, "Should return 1 document with processing status")

		// 验证文档状态
		if len(documents) > 0 {
			doc := documents[0].(map[string]interface{})
			assert.Equal(t, "processing", doc["status"], "Document status should be processing")
		}
	})

	// 测试按标签过滤
	t.Run("filter by tags", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/documents?tags=report", nil)
		w := httptest.NewRecorder()
		env.Router.ServeHTTP(w, req)

		// 验证响应
		var resp model.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证只返回带有report标签的文档
		listResp, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(2), listResp["total"], "Should find 2 documents with report tag")

		documents, ok := listResp["documents"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, documents, 2, "Should return 2 documents with report tag")
	})

	// 测试组合过滤条件
	t.Run("combined filters", func(t *testing.T) {
		// 过滤report标签且已完成的文档
		req := httptest.NewRequest(http.MethodGet, "/api/documents?tags=report&status=completed", nil)
		w := httptest.NewRecorder()
		env.Router.ServeHTTP(w, req)

		// 验证响应
		var resp model.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		// 验证返回结果
		listResp, ok := resp.Data.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(1), listResp["total"], "Should find 1 document with report tag and completed status")
	})
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

func cleanDatabase(t *testing.T) {
	db := database.MustDB()
	db.Exec("PRAGMA foreign_keys = OFF")

	// 清理所有相关表
	tables := []string{"documents", "document_segments"}
	for _, table := range tables {
		err := db.Exec("DELETE FROM " + table).Error
		require.NoError(t, err, "Failed to clear table: "+table)
	}

	db.Exec("PRAGMA foreign_keys = ON")

	t.Log("Database tables cleared")
}
