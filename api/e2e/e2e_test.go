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
	TempDir         string
	BaseURL         string
	CleanupFuncs    []func()
}

// 设置测试环境
func setupE2ETestEnv(t *testing.T) *e2eTestEnv {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa_e2e_*")
	require.NoError(t, err)

	// 创建测试环境
	env := &e2eTestEnv{
		TempDir:      tempDir,
		CleanupFuncs: []func(){},
	}

	// 添加目录清理函数
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		os.RemoveAll(tempDir)
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

	// 设置FAISS向量数据库（不使用内存实现）
	faissIndexPath := filepath.Join(tempDir, "faiss_index")
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:              "faiss",
		Path:              faissIndexPath,
		Dimension:         1536,
		DistanceType:      vectordb.Cosine,
		CreateIfNotExists: true,
	})

	if err != nil {
		t.Fatalf("Failed to create FAISS vector database: %v", err)
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
		mock.Anything,   // 上下文参数
		mock.Anything,   // 提示词
		mock.Anything,   // 第一个选项参数
		mock.Anything,   // 第二个选项参数,
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

	// 创建文档服务
	env.DocumentService = services.NewDocumentService(
		env.Storage,
		nil, // 使用ParserFactory
		splitter,
		mockEmbedding,
		vectorDB,
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

// 创建测试文件
func createTestFile(t *testing.T, filename, content string) string {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)
	return filePath
}

// TestDocumentUploadAndQuery 测试文档上传和查询功能
func TestDocumentUploadAndQuery(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// 测试文档内容
	testContent := "这是一个关于向量数据库的测试文档。向量数据库用于存储和检索向量数据。"
	testFile := createTestFile(t, "test.txt", testContent)

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
		assert.Equal(t, "processing", response.Data.Status)

		// 存储文件ID用于后续测试
		fileID := response.Data.FileID
		t.Logf("Uploaded file ID: %s", fileID)

		// 等待文档处理完成
		time.Sleep(2 * time.Second)

		// 第2步：发送问题查询
		t.Run("QueryDocument", func(t *testing.T) {
			// 准备查询请求
			queryData := map[string]interface{}{
				"question": "向量数据库是什么?",
				"file_id":  fileID,
			}
			queryJSON, err := json.Marshal(queryData)
			require.NoError(t, err)

			// 发送问答请求
			qaResp, err := http.Post(
				fmt.Sprintf("%s/api/qa", env.BaseURL),
				"application/json",
				bytes.NewBuffer(queryJSON),
			)
			require.NoError(t, err)
			defer qaResp.Body.Close()

			// 检查状态码
			assert.Equal(t, http.StatusOK, qaResp.StatusCode)

			// 解析响应
			var qaResponse struct {
				Code    int              `json:"code"`
				Message string           `json:"message"`
				Data    model.QAResponse `json:"data"`
			}
			err = json.NewDecoder(qaResp.Body).Decode(&qaResponse)
			require.NoError(t, err)

			// 验证响应
			assert.Equal(t, 0, qaResponse.Code)
			assert.Equal(t, "向量数据库是什么?", qaResponse.Data.Question)
			assert.NotEmpty(t, qaResponse.Data.Answer)
		})

		// 第3步：删除文档
		t.Run("DeleteDocument", func(t *testing.T) {
			// 创建删除请求
			req, err := http.NewRequest(
				"DELETE",
				fmt.Sprintf("%s/api/documents/%s", env.BaseURL, fileID),
				nil,
			)
			require.NoError(t, err)

			// 发送请求
			client := &http.Client{}
			delResp, err := client.Do(req)
			require.NoError(t, err)
			defer delResp.Body.Close()

			// 检查状态码
			assert.Equal(t, http.StatusOK, delResp.StatusCode)

			// 解析响应
			var delResponse struct {
				Code    int                          `json:"code"`
				Message string                       `json:"message"`
				Data    model.DocumentDeleteResponse `json:"data"`
			}
			err = json.NewDecoder(delResp.Body).Decode(&delResponse)
			require.NoError(t, err)

			// 验证响应
			assert.Equal(t, 0, delResponse.Code)
			assert.Equal(t, true, delResponse.Data.Success)
			assert.Equal(t, fileID, delResponse.Data.FileID)
		})
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
	}{
		{"golang.txt", "Go是一种静态类型的编译语言，具有垃圾收集功能。Go的并发特性非常强大。"},
		{"python.txt", "Python是一种解释型高级编程语言，以其简洁的语法和丰富的库而闻名。"},
	}

	var fileIDs []string

	// 上传文档
	for _, doc := range documents {
		filePath := createTestFile(t, doc.name, doc.content)

		// 创建multipart请求
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", doc.name)
		require.NoError(t, err)

		file, err := os.Open(filePath)
		require.NoError(t, err)

		_, err = io.Copy(part, file)
		require.NoError(t, err)
		file.Close()
		writer.Close()

		// 发送请求
		resp, err := http.Post(
			fmt.Sprintf("%s/api/documents", env.BaseURL),
			writer.FormDataContentType(),
			body,
		)
		require.NoError(t, err)

		// 解析响应
		var response struct {
			Data struct {
				FileID string `json:"file_id"`
			} `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)
		resp.Body.Close()

		fileIDs = append(fileIDs, response.Data.FileID)
	}

	// 等待文档处理完成
	time.Sleep(2 * time.Second)

	// 查询第一个文档
	t.Run("QuerySpecificDocument", func(t *testing.T) {
		if len(fileIDs) == 0 {
			t.Skip("No documents uploaded")
		}

		queryData := map[string]interface{}{
			"question": "Go语言有什么特点?",
			"file_id":  fileIDs[0],
		}
		queryJSON, err := json.Marshal(queryData)
		require.NoError(t, err)

		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(queryJSON),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var qaResponse struct {
			Data model.QAResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&qaResponse)
		require.NoError(t, err)

		assert.Equal(t, "Go语言有什么特点?", qaResponse.Data.Question)
		assert.NotEmpty(t, qaResponse.Data.Answer)
	})

	// 不指定文档ID的一般性查询
	t.Run("GeneralQuery", func(t *testing.T) {
		queryData := map[string]interface{}{
			"question": "编程语言的特点是什么?",
		}
		queryJSON, err := json.Marshal(queryData)
		require.NoError(t, err)

		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(queryJSON),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var qaResponse struct {
			Data model.QAResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&qaResponse)
		require.NoError(t, err)

		assert.Equal(t, "编程语言的特点是什么?", qaResponse.Data.Question)
		assert.NotEmpty(t, qaResponse.Data.Answer)
	})

	// 清理测试文档
	for _, fileID := range fileIDs {
		req, _ := http.NewRequest(
			"DELETE",
			fmt.Sprintf("%s/api/documents/%s", env.BaseURL, fileID),
			nil,
		)
		client := &http.Client{}
		resp, _ := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	env := setupE2ETestEnv(t)
	defer env.cleanup()

	// 测试空问题
	t.Run("EmptyQuestion", func(t *testing.T) {
		queryData := map[string]interface{}{
			"question": "",
		}
		queryJSON, err := json.Marshal(queryData)
		require.NoError(t, err)

		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(queryJSON),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 应该返回错误
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errorResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		require.NoError(t, err)

		assert.NotEqual(t, 0, errorResp.Code) // 非零表示错误
		assert.NotEmpty(t, errorResp.Message) // 应该有错误消息
	})

	// 测试上传不支持的文件类型
	t.Run("UnsupportedFileType", func(t *testing.T) {
		// 创建一个不支持的文件类型
		unsupportedFile := createTestFile(t, "test.xyz", "测试内容")

		// 创建multipart请求
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.xyz")
		require.NoError(t, err)

		file, err := os.Open(unsupportedFile)
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

		var errorResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		require.NoError(t, err)

		assert.NotEqual(t, 0, errorResp.Code)
		assert.Contains(t, errorResp.Message, "不支持的文件类型")
	})

	// 测试查询不存在的文档
	t.Run("NonExistentDocument", func(t *testing.T) {
		queryData := map[string]interface{}{
			"question": "这是一个测试问题",
			"file_id":  "non-existent-id",
		}
		queryJSON, err := json.Marshal(queryData)
		require.NoError(t, err)

		resp, err := http.Post(
			fmt.Sprintf("%s/api/qa", env.BaseURL),
			"application/json",
			bytes.NewBuffer(queryJSON),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		// 检查响应
		var qaResponse struct {
			Data model.QAResponse `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&qaResponse)
		require.NoError(t, err)

		// 应当有回答（由于mock）
		assert.NotEmpty(t, qaResponse.Data.Answer)
	})
}
