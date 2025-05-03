package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// 端到端测试环境
type e2eTestEnv struct {
	Router          *gin.Engine
	Server          *httptest.Server
	Storage         storage.Storage
	VectorDB        vectordb.Repository
	EmbeddingClient embedding.Client
	LLMClient       llm.Client
	DocumentService *services.DocumentService
	QAService       *services.QAService
	StatusManager   *services.DocumentStatusManager
	Repository      repository.DocumentRepository
	TempDir         string
	BaseURL         string
	Logger          *logrus.Logger
	CleanupFuncs    []func()
}

// 设置测试环境
func setupE2ETestEnv(t *testing.T) *e2eTestEnv {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa_e2e_*")
	require.NoError(t, err)

	// 创建日志
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// 创建测试环境
	env := &e2eTestEnv{
		TempDir:      tempDir,
		CleanupFuncs: []func(){},
		Logger:       logger,
	}

	// 添加目录清理函数
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		os.RemoveAll(tempDir)
	})

	// 初始化SQLite数据库 - 新增部分
	dbPath := filepath.Join(tempDir, "docqa_test.db")
	dbConfig := &database.Config{
		Type: "sqlite",
		DSN:  dbPath,
	}
	err = database.Setup(dbConfig, logger)
	require.NoError(t, err, "Failed to setup database")

	// 添加数据库清理函数
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		database.Close()
		os.Remove(dbPath)
	})

	// 尝试使用MinIO存储
	minioStorage, err := storage.NewMinioStorage(storage.MinioConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
		Bucket:    "docqa-test",
	})

	if err != nil {
		// MinIO不可用，回退到本地存储
		t.Logf("MinIO not available, falling back to local storage: %v", err)
		fileStorage, err := storage.NewLocalStorage(storage.LocalConfig{
			Path: tempDir,
		})
		require.NoError(t, err)
		env.Storage = fileStorage
	} else {
		env.Storage = minioStorage
		t.Log("Using MinIO storage")
	}

	// 设置Redis缓存
	var cacheService cache.Cache
	redisConfig := cache.Config{
		Type:       "redis",
		RedisAddr:  "localhost:6379", // 假设Redis在本地默认端口运行
		DefaultTTL: time.Hour,
	}

	cacheService, err = cache.NewCache(redisConfig)
	if err != nil {
		// Redis不可用，回退到内存缓存
		t.Logf("Redis not available, falling back to memory cache: %v", err)
		memoryConfig := cache.Config{
			Type:       "memory",
			DefaultTTL: time.Hour,
		}
		cacheService, err = cache.NewCache(memoryConfig)
		require.NoError(t, err)
	} else {
		t.Log("Using Redis cache")
	}

	// 设置FAISS向量数据库
	faissIndexPath := filepath.Join(tempDir, "faiss_index")
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:              "faiss",
		Path:              faissIndexPath,
		Dimension:         1536,
		DistanceType:      vectordb.Cosine,
		CreateIfNotExists: true,
	})

	if err != nil {
		t.Logf("Failed to create FAISS vector database: %v", err)
		t.Log("Falling back to in-memory vector database")

		vectorDB, err = vectordb.NewRepository(vectordb.Config{
			Type:         "memory",
			Dimension:    1536,
			DistanceType: vectordb.Cosine,
		})
		require.NoError(t, err)
	}

	env.VectorDB = vectorDB
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		vectorDB.Close()
	})

	// 设置Mock嵌入客户端
	mockEmbedding := embedding.NewMockClient(t)
	mockEmbedding.On("Name").Return("mock-embedding").Maybe()
	mockEmbedding.On("Embed", mock.Anything, mock.Anything).Return(
		make([]float32, 1536), nil,
	).Maybe()

	// 创建一个固定大小的返回值数组
	staticEmbeddings := make([][]float32, 5) // 预设支持最多5个文本片段
	for i := range staticEmbeddings {
		staticEmbeddings[i] = make([]float32, 1536)
	}

	// 直接返回静态数组，而不是返回函数
	mockEmbedding.On("EmbedBatch", mock.Anything, mock.Anything).Return(
		staticEmbeddings, nil,
	).Maybe()

	env.EmbeddingClient = mockEmbedding

	// 设置Mock LLM客户端
	mockLLM := llm.NewMockClient(t)
	mockLLM.On("Name").Return("mock-llm").Maybe()
	mockLLM.On("Generate",
		mock.Anything, // 上下文参数
		mock.Anything, // 提示词
		mock.Anything, // 第一个选项参数
		mock.Anything, // 第二个选项参数,
	).Return(
		&llm.Response{
			Text:       "这是测试回答",
			TokenCount: 10,
			ModelName:  "mock-model",
			FinishTime: time.Now(),
		},
		nil,
	).Maybe()
	env.LLMClient = mockLLM

	// 创建文本分段器
	splitter := document.NewTextSplitter(document.SplitterConfig{
		SplitType:    document.ByParagraph,
		ChunkSize:    500,
		ChunkOverlap: 100,
	})

	// 创建RAG服务
	ragService := llm.NewRAG(mockLLM,
		llm.WithRAGMaxTokens(1024),
		llm.WithRAGTemperature(0.7),
	)

	// 初始化文档仓储 - 新增部分
	repo := repository.NewDocumentRepository()
	env.Repository = repo

	// 初始化文档状态管理器 - 新增部分
	statusManager := services.NewDocumentStatusManager(repo, logger)
	env.StatusManager = statusManager

	// 创建文档服务
	env.DocumentService = services.NewDocumentService(
		env.Storage,
		nil, // 使用ParserFactory
		splitter,
		mockEmbedding,
		vectorDB,
		services.WithStatusManager(statusManager), // 使用状态管理器
		services.WithBatchSize(5),
	)

	// 创建问答服务
	env.QAService = services.NewQAService(
		mockEmbedding,
		vectorDB,
		mockLLM,
		ragService,
		cacheService,
		services.WithMinScore(0.0), // 设置为0以便于测试
	)

	// 设置API处理器
	docHandler := handler.NewDocumentHandler(env.DocumentService, env.Storage)
	qaHandler := handler.NewQAHandler(env.QAService)

	// 设置路由
	router := gin.Default()
	api := router.Group("/api")
	{
		// 文档相关路由
		api.POST("/documents", docHandler.UploadDocument)
		api.GET("/documents/:id/status", docHandler.GetDocumentStatus)
		api.GET("/documents", docHandler.ListDocuments)
		api.DELETE("/documents/:id", docHandler.DeleteDocument)
		api.PATCH("/documents/:id", docHandler.UpdateDocument) // 新增的路由 - 更新文档信息

		// 问答相关路由
		api.POST("/qa", qaHandler.AnswerQuestion)
	}

	env.Router = router

	// 创建测试服务器
	server := httptest.NewServer(router)
	env.Server = server
	env.BaseURL = server.URL
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		server.Close()
	})

	return env
}

// 清理测试环境
func (env *e2eTestEnv) cleanup() {
	for _, cleanupFn := range env.CleanupFuncs {
		cleanupFn()
	}
}

// createTestFile 创建测试文件
func createTestFile(t *testing.T, filename, content string) string {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)
	return filePath
}

// TestDocumentLifecycle 测试文档生命周期
func TestDocumentLifecycle(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// 测试文档内容
	testContent := "这是一个关于向量数据库的测试文档。向量数据库用于存储和检索向量数据。"
	testFile := createTestFile(t, "test.txt", testContent)

	var fileID string

	// 第1步：上传文档
	t.Run("UploadDocument", func(t *testing.T) {
		// 创建multipart请求
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)

		file, err := os.Open(testFile)
		require.NoError(t, err)
		defer file.Close()

		_, err = io.Copy(part, file)
		require.NoError(t, err)

		// 添加标签 - 新增测试特性
		fmt.Printf("DEBUG: Setting tags to %q in upload request\n", "test,vector,database")
		_ = writer.WriteField("tags", "test,vector,database")
		fmt.Printf("DEBUG: Form contains tags field: %v\n", writer.FormDataContentType())

		writer.Close()

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/documents", env.BaseURL),
			writer.FormDataContentType(),
			body,
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int                          `json:"code"`
			Message string                       `json:"message"`
			Data    model.DocumentUploadResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证响应
		assert.Equal(t, 0, response.Code)
		assert.NotEmpty(t, response.Data.FileID)
		assert.Equal(t, "test.txt", response.Data.FileName)
		assert.Equal(t, "uploaded", response.Data.Status)

		// 存储文件ID用于后续测试
		fileID = response.Data.FileID
		t.Logf("Uploaded file ID: %s", fileID)
	})

	// 第2步：检查文档状态
	t.Run("CheckDocumentStatus", func(t *testing.T) {
		// 等待文档处理完成
		time.Sleep(2 * time.Second)

		// 发送获取状态请求
		resp, err := http.Get(fmt.Sprintf("%s/api/documents/%s/status", env.BaseURL, fileID))
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int                          `json:"code"`
			Message string                       `json:"message"`
			Data    model.DocumentStatusResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证响应 - 记录任何状态，因为处理可能尚未完成
		t.Logf("Document status: %s", response.Data.Status)
		assert.Equal(t, 0, response.Code)
		assert.Equal(t, fileID, response.Data.FileID)
		assert.Equal(t, "test.txt", response.Data.FileName)
		assert.Contains(t, []string{"uploaded", "processing", "completed"}, response.Data.Status)

		// 检查标签 - 新增测试特性
		assert.Equal(t, "test,vector,database", response.Data.Tags)
	})

	// 第3步：测试文档列表功能 - 新增测试用例
	t.Run("ListDocuments", func(t *testing.T) {
		// 发送获取文档列表请求
		resp, err := http.Get(fmt.Sprintf("%s/api/documents", env.BaseURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int                        `json:"code"`
			Message string                     `json:"message"`
			Data    model.DocumentListResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证响应
		assert.Equal(t, 0, response.Code)
		assert.Equal(t, int64(1), response.Data.Total) // 应该只有一个文档
		assert.Equal(t, 1, response.Data.Page)
		assert.Len(t, response.Data.Documents, 1)
		assert.Equal(t, fileID, response.Data.Documents[0].FileID)
	})

	// 第4步：测试标签过滤功能 - 新增测试用例
	t.Run("FilterDocumentsByTag", func(t *testing.T) {
		// 发送带有标签过滤条件的请求
		resp, err := http.Get(fmt.Sprintf("%s/api/documents?tags=vector", env.BaseURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int                        `json:"code"`
			Message string                     `json:"message"`
			Data    model.DocumentListResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证过滤响应
		assert.Equal(t, 0, response.Code)
		assert.Equal(t, int64(1), response.Data.Total) // 应有1个匹配的文档
		assert.Len(t, response.Data.Documents, 1)

		// 测试不匹配的标签
		resp, err = http.Get(fmt.Sprintf("%s/api/documents?tags=nonexistent", env.BaseURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, int64(0), response.Data.Total) // 应该没有匹配的文档
	})

	// 第5步：发送问题查询
	t.Run("AnswerQuestion", func(t *testing.T) {
		// 准备请求体
		reqBody := map[string]interface{}{
			"question": "什么是向量数据库？",
			"file_id":  fileID,
		}
		jsonData, err := json.Marshal(reqBody)
		require.NoError(t, err)

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int              `json:"code"`
			Message string           `json:"message"`
			Data    model.QAResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证响应
		assert.Equal(t, 0, response.Code)
		assert.NotEmpty(t, response.Data.Answer)
		assert.Equal(t, "什么是向量数据库？", response.Data.Question)
	})

	// 第6步：更新文档标签 - 新增测试用例
	t.Run("UpdateDocumentTags", func(t *testing.T) {
		// 准备更新请求
		updateReq := map[string]interface{}{
			"tags": "updated,vector,test",
		}
		jsonData, err := json.Marshal(updateReq)
		require.NoError(t, err)

		// 创建PATCH请求
		req, err := http.NewRequest(
			http.MethodPatch,
			fmt.Sprintf("%s/api/documents/%s", env.BaseURL, fileID),
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// 发送请求
		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 获取文档状态验证标签是否更新
		respStatus, err := http.Get(fmt.Sprintf("%s/api/documents/%s/status", env.BaseURL, fileID))
		require.NoError(t, err)
		defer respStatus.Body.Close()

		var statusResp struct {
			Code    int                          `json:"code"`
			Message string                       `json:"message"`
			Data    model.DocumentStatusResponse `json:"data"`
		}
		err = json.NewDecoder(respStatus.Body).Decode(&statusResp)
		require.NoError(t, err)

		// 验证标签已更新
		assert.Equal(t, "updated,vector,test", statusResp.Data.Tags)
	})

	// 第7步：删除文档
	t.Run("DeleteDocument", func(t *testing.T) {
		// 创建DELETE请求
		req, err := http.NewRequest(
			http.MethodDelete,
			fmt.Sprintf("%s/api/documents/%s", env.BaseURL, fileID),
			nil,
		)
		require.NoError(t, err)

		// 发送请求
		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int                          `json:"code"`
			Message string                       `json:"message"`
			Data    model.DocumentDeleteResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证响应
		assert.Equal(t, 0, response.Code)
		assert.True(t, response.Data.Success)
		assert.Equal(t, fileID, response.Data.FileID)

		// 验证文档已被删除
		respCheck, err := http.Get(fmt.Sprintf("%s/api/documents/%s/status", env.BaseURL, fileID))
		require.NoError(t, err)
		defer respCheck.Body.Close()
		assert.Equal(t, http.StatusNotFound, respCheck.StatusCode)
	})
}

// TestMultipleDocumentSearch 测试多文档搜索
func TestMultipleDocumentSearch(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// 上传两个不同的文档
	documents := []struct {
		name    string
		content string
		tags    string
	}{
		{"golang.txt", "Go是一种静态类型的编译语言，具有垃圾收集功能。Go的并发特性非常强大。", "programming,golang"},
		{"python.txt", "Python是一种解释型高级编程语言，以其简洁的语法和丰富的库而闻名。", "programming,python"},
	}

	var fileIDs []string

	// 上传文档
	for _, doc := range documents {
		// 创建临时文件
		testFile := createTestFile(t, doc.name, doc.content)

		// 创建multipart请求
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", doc.name)
		require.NoError(t, err)

		file, err := os.Open(testFile)
		require.NoError(t, err)
		defer file.Close()

		_, err = io.Copy(part, file)
		require.NoError(t, err)

		// 添加标签
		_ = writer.WriteField("tags", doc.tags)

		writer.Close()

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/documents", env.BaseURL),
			writer.FormDataContentType(),
			body,
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 解析响应
		var response struct {
			Code    int                          `json:"code"`
			Message string                       `json:"message"`
			Data    model.DocumentUploadResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		fileIDs = append(fileIDs, response.Data.FileID)
		t.Logf("Uploaded file ID for %s: %s", doc.name, response.Data.FileID)
	}

	// 等待文档处理完成
	time.Sleep(2 * time.Second)

	// 查询第一个文档
	t.Run("QuerySpecificDocument", func(t *testing.T) {
		// 准备请求体
		reqBody := map[string]interface{}{
			"question": "Go语言有什么特点？",
			"file_id":  fileIDs[0],
		}
		jsonData, err := json.Marshal(reqBody)
		require.NoError(t, err)

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 由于使用了Mock LLM，所以结果内容不重要，只确认流程正常
	})

	// 不指定文档ID的一般性查询
	t.Run("GeneralQuery", func(t *testing.T) {
		// 准备请求体
		reqBody := map[string]interface{}{
			"question": "编程语言有哪些？",
		}
		jsonData, err := json.Marshal(reqBody)
		require.NoError(t, err)

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// 测试元数据过滤 - 新增测试用例
	t.Run("MetadataFilter", func(t *testing.T) {
		// 准备请求体
		reqBody := map[string]interface{}{
			"question": "Python的特点是什么？",
			"metadata": map[string]interface{}{
				"tags": "python",
			},
		}
		jsonData, err := json.Marshal(reqBody)
		require.NoError(t, err)

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查状态码
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 由于使用了Mock LLM，这里不检查具体回答内容
	})

	// 测试文档列表分页和过滤 - 新增测试用例
	t.Run("ListWithPagination", func(t *testing.T) {
		// 测试分页
		resp, err := http.Get(fmt.Sprintf("%s/api/documents?page=1&page_size=1", env.BaseURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		var response struct {
			Code    int                        `json:"code"`
			Message string                     `json:"message"`
			Data    model.DocumentListResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证分页
		assert.Equal(t, int64(2), response.Data.Total) // 总共有2个文档
		assert.Equal(t, 1, response.Data.Page)
		assert.Equal(t, 1, response.Data.PageSize)
		assert.Len(t, response.Data.Documents, 1) // 但因为分页只返回1个

		// 测试标签过滤
		resp, err = http.Get(fmt.Sprintf("%s/api/documents?tags=python", env.BaseURL))
		require.NoError(t, err)
		defer resp.Body.Close()

		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// 验证过滤
		assert.Equal(t, int64(1), response.Data.Total) // 只有1个包含python标签
	})

	// 清理测试文档
	for _, fileID := range fileIDs {
		req, err := http.NewRequest(
			http.MethodDelete,
			fmt.Sprintf("%s/api/documents/%s", env.BaseURL, fileID),
			nil,
		)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// 测试空问题
	t.Run("EmptyQuestion", func(t *testing.T) {
		// 准备请求体（空问题）
		reqBody := map[string]interface{}{
			"question": "",
		}
		jsonData, err := json.Marshal(reqBody)
		require.NoError(t, err)

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回错误
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var response model.Response
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.NotEqual(t, 0, response.Code) // 非零表示错误
		assert.NotEmpty(t, response.Message) // 应该有错误消息
	})

	// 测试上传不支持的文件类型
	t.Run("UnsupportedFileType", func(t *testing.T) {
		// 创建一个不支持的文件类型
		testFile := createTestFile(t, "test.xyz", "测试内容")

		// 创建multipart请求
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.xyz")
		require.NoError(t, err)

		file, err := os.Open(testFile)
		require.NoError(t, err)
		defer file.Close()

		_, err = io.Copy(part, file)
		require.NoError(t, err)
		writer.Close()

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/documents", env.BaseURL),
			writer.FormDataContentType(),
			body,
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回错误
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	// 测试查询不存在的文档
	t.Run("NonExistentDocument", func(t *testing.T) {
		// 准备请求体
		reqBody := map[string]interface{}{
			"question": "什么是向量数据库？",
			"file_id":  "non-existent-id",
		}
		jsonData, err := json.Marshal(reqBody)
		require.NoError(t, err)

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查响应
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		var response model.Response
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.NotEqual(t, 0, response.Code)
	})
}
