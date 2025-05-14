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

	"github.com/fyerfyer/doc-QA-system/api"
	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pythonIntegrationEnv Python集成测试环境
type pythonIntegrationEnv struct {
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
	TaskQueue       taskqueue.Queue
	TempDir         string
	BaseURL         string
	Logger          *logrus.Logger
	CleanupFuncs    []func()
}

// 设置Python集成测试环境
func setupPythonTestEnv(t *testing.T) *pythonIntegrationEnv {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 加载环境变量
	err := godotenv.Load("../../.env")
	if err != nil {
		t.Logf("Warning: Error loading .env file: %v", err)
	}

	// 获取API密钥
	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		t.Fatalf("DASHSCOPE_API_KEY environment variable not set")
	}
	t.Logf("Successfully loaded API key")

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa_py_integration_*")
	require.NoError(t, err, "Failed to create temporary directory")

	// 创建日志实例
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// 创建测试环境
	env := &pythonIntegrationEnv{
		TempDir:      tempDir,
		CleanupFuncs: []func(){},
		Logger:       logger,
	}

	// 添加目录清理函数
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		os.RemoveAll(tempDir)
	})

	// 初始化SQLite数据库
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
	})

	// 初始化MinIO存储
	minioStorage, err := storage.NewMinioStorage(storage.MinioConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
		Bucket:    "docqa-test",
	})
	require.NoError(t, err, "Failed to create MinIO storage")

	env.Storage = minioStorage

	// 设置Redis缓存
	redisCache, err := cache.NewCache(cache.Config{
		Type:       "redis",
		RedisAddr:  "localhost:6379", // Redis在本地默认端口运行
		DefaultTTL: time.Hour,
	})
	require.NoError(t, err, "Failed to connect to Redis")

	// 设置FAISS向量数据库
	faissIndexPath := filepath.Join(tempDir, "faiss_index")
	vectorDB, err := vectordb.NewRepository(vectordb.Config{
		Type:              "faiss",
		Path:              faissIndexPath,
		Dimension:         1536,
		DistanceType:      vectordb.Cosine,
		CreateIfNotExists: true,
	})
	require.NoError(t, err, "Failed to create FAISS vector database")

	env.VectorDB = vectorDB
	env.CleanupFuncs = append(env.CleanupFuncs, func() {
		vectorDB.Close()
	})

	// 创建真实的通义千问嵌入客户端
	tongyiEmbedder, err := embedding.NewClient("tongyi",
		embedding.WithAPIKey(apiKey),
		embedding.WithDimensions(1536),
	)
	require.NoError(t, err, "Failed to create Tongyi embedding client")

	env.EmbeddingClient = tongyiEmbedder

	// 创建真实的通义千问LLM客户端
	tongyiLLM, err := llm.NewClient("tongyi",
		llm.WithAPIKey(apiKey),
		llm.WithModel(llm.ModelQwenTurbo),
	)
	require.NoError(t, err, "Failed to create Tongyi LLM client")

	env.LLMClient = tongyiLLM

	// 创建任务队列
	queueConfig := taskqueue.DefaultConfig()
	queueConfig.RedisAddr = "localhost:6379"
	queue, err := taskqueue.NewQueue("redis", queueConfig)
	require.NoError(t, err, "Failed to create task queue")
	env.TaskQueue = queue

	// 创建文档仓储
	repo := repository.NewDocumentRepositoryWithQueue(database.DB, queue)
	env.Repository = repo

	// 创建文档状态管理器
	statusManager := services.NewDocumentStatusManager(repo, logger)
	env.StatusManager = statusManager

	// 创建RAG服务
	ragService := llm.NewRAG(tongyiLLM,
		llm.WithRAGMaxTokens(1024),
		llm.WithRAGTemperature(0.7),
	)

	// 创建问答服务
	qaService := services.NewQAService(
		tongyiEmbedder,
		vectorDB,
		tongyiLLM,
		ragService,
		redisCache,
		services.WithSearchLimit(3),
		services.WithMinScore(0.5),
	)
	env.QAService = qaService

	// 创建文档服务
	documentService := services.NewDocumentService(
		env.Storage,
		nil, // 使用ParserFactory
		nil, // 使用Python服务进行分块
		tongyiEmbedder,
		vectorDB,
		services.WithStatusManager(statusManager),
		services.WithTaskQueue(queue),
		services.WithAsyncProcessing(true),
		services.WithBatchSize(5),
		services.WithDocumentRepository(repo),
		services.WithLogger(logger),
	)
	env.DocumentService = documentService

	// 创建API处理器
	docHandler := handler.NewDocumentHandler(documentService, env.Storage)
	qaHandler := handler.NewQAHandler(qaService)
	taskHandler := handler.NewTaskHandler(queue)

	// 设置路由
	router := api.SetupRouter(docHandler, qaHandler)
	api.RegisterTaskRoutes(router, taskHandler)
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
func (env *pythonIntegrationEnv) cleanup() {
	for _, cleanup := range env.CleanupFuncs {
		cleanup()
	}
}

// createTestFile 创建测试文件
func createPyTestFile(t *testing.T, dir, filename, content string) string {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create test file")
	return filePath
}

// TestPythonIntegration 测试Python服务集成
func TestPythonIntegration(t *testing.T) {
	// 跳过测试如果未设置集成测试标志
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test; set RUN_INTEGRATION_TESTS=true to run")
	}

	// 设置测试环境
	env := setupPythonTestEnv(t)
	defer env.cleanup()

	// 测试文档内容
	testContent := `# 测试文档
这是一个测试文档，用于Python服务集成测试。

## 概述
本文档包含一些信息，用于测试文档解析、分块和向量化功能。

## 主要功能
1. 文档解析：将原始文件转换为纯文本
2. 文本分块：将文本分割成语义块
3. 向量化：为每个文本块生成向量表示

## 测试目标
测试Python服务与Go服务之间的集成功能，确保完整的文档处理流程正常工作。`

	// 创建测试文件
	testFileName := "test_integration.md"
	testFilePath := createPyTestFile(t, env.TempDir, testFileName, testContent)

	// 第1步：测试文档上传
	t.Log("Step 1: Testing document upload")
	fileID := uploadDocument(t, env, testFilePath, testFileName)
	require.NotEmpty(t, fileID, "File ID should not be empty")

	// 第2步：等待文档处理完成
	t.Log("Step 2: Waiting for document processing")
	waitForDocumentProcessing(t, env, fileID)

	// 第3步：检查文档状态
	t.Log("Step 3: Checking document status")
	docStatus, _ := getDocumentStatus(t, env, fileID)
	assert.Equal(t, "completed", docStatus.Status, "Document should be in completed status")
	assert.True(t, docStatus.Segments > 0, "Document should have segments")

	// 第4步：发送问题查询
	t.Log("Step 4: Testing question answering")
	question := "什么是文本分块？"
	answer := askQuestion(t, env, question, fileID)
	assert.NotEmpty(t, answer.Answer, "Answer should not be empty")
	assert.True(t, len(answer.Sources) > 0, "Sources should not be empty")
	t.Logf("Question: %s", question)
	t.Logf("Answer: %s", answer.Answer)
	t.Logf("Sources count: %d", len(answer.Sources))

	// 第5步：测试不指定文档ID的通用问答
	t.Log("Step 5: Testing general QA without specific document")
	generalQuestion := "文档处理流程包括哪些步骤？"
	generalAnswer := askQuestion(t, env, generalQuestion, "")
	assert.NotEmpty(t, generalAnswer.Answer, "Answer should not be empty")
	t.Logf("General Question: %s", generalQuestion)
	t.Logf("General Answer: %s", generalAnswer.Answer)

	// 第6步：删除文档
	t.Log("Step 6: Testing document deletion")
	deleteDocument(t, env, fileID)

	// 检查文档是否已删除
	resp, err := http.Get(fmt.Sprintf("%s/api/documents/%s/status", env.BaseURL, fileID))
	require.NoError(t, err, "HTTP request should not fail")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Document should be deleted")
}

// TestPythonServiceError 测试Python服务错误处理
func TestPythonServiceError(t *testing.T) {
	// 跳过测试如果未设置集成测试标志
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test; set RUN_INTEGRATION_TESTS=true to run")
	}

	// 设置测试环境
	env := setupPythonTestEnv(t)
	defer env.cleanup()

	// 测试上传不支持的文件类型
	t.Log("Testing upload of unsupported file type")

	// 创建一个不支持的文件类型
	unsupportedFileName := "test.xyz"
	unsupportedContent := "This is an unsupported file type"
	unsupportedFilePath := createPyTestFile(t, env.TempDir, unsupportedFileName, unsupportedContent)

	// 上传不支持的文件
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 添加文件
	file, err := os.Open(unsupportedFilePath)
	require.NoError(t, err, "Failed to open test file")
	defer file.Close()

	part, err := writer.CreateFormFile("file", unsupportedFileName)
	require.NoError(t, err, "Failed to create form file")

	_, err = io.Copy(part, file)
	require.NoError(t, err, "Failed to copy file content")

	// 完成multipart写入
	writer.Close()

	// 发送请求
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/documents", env.BaseURL), &buf)
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should not fail")
	defer resp.Body.Close()

	// 验证响应状态码（应该是400 Bad Request，因为文件类型不支持）
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return bad request for unsupported file type")
}

// uploadDocument 上传文档并返回文档ID
func uploadDocument(t *testing.T, env *pythonIntegrationEnv, filePath string, fileName string) string {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 添加文件
	file, err := os.Open(filePath)
	require.NoError(t, err, "Failed to open test file")
	defer file.Close()

	part, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err, "Failed to create form file")

	_, err = io.Copy(part, file)
	require.NoError(t, err, "Failed to copy file content")

	// 添加标签
	err = writer.WriteField("tags", "integration,test,python")
	require.NoError(t, err, "Failed to write tags field")

	// 完成multipart写入
	writer.Close()

	// 发送请求
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/documents", env.BaseURL), &buf)
	require.NoError(t, err, "Failed to create request")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should not fail")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Upload should succeed")

	var response struct {
		Code int `json:"code"`
		Data struct {
			FileID string `json:"file_id"`
		} `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err, "Failed to decode response")

	return response.Data.FileID
}

// waitForDocumentProcessing 等待文档处理完成
func waitForDocumentProcessing(t *testing.T, env *pythonIntegrationEnv, fileID string) {
	// 等待最长2分钟
	timeout := time.After(2 * time.Minute)
	tick := time.Tick(5 * time.Second)

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for document processing")
			return
		case <-tick:
			status, err := getDocumentStatus(t, env, fileID)
			require.NoError(t, err, "Failed to get document status")

			t.Logf("Document status: %s, progress: %d", status.Status, status.Progress)

			if status.Status == "completed" {
				return
			} else if status.Status == "failed" {
				t.Fatalf("Document processing failed: %s", status.Error)
				return
			}
		}
	}
}

// getDocumentStatus 获取文档状态
func getDocumentStatus(t *testing.T, env *pythonIntegrationEnv, fileID string) (*model.DocumentStatusResponse, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/documents/%s/status", env.BaseURL, fileID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Status request should succeed")

	var response struct {
		Code int                          `json:"code"`
		Data model.DocumentStatusResponse `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	return &response.Data, nil
}

// askQuestion 发送问题查询
func askQuestion(t *testing.T, env *pythonIntegrationEnv, question string, fileID string) model.QAResponse {
	requestBody := map[string]interface{}{
		"question": question,
	}

	if fileID != "" {
		requestBody["file_id"] = fileID
	}

	jsonData, err := json.Marshal(requestBody)
	require.NoError(t, err, "Failed to marshal request")

	resp, err := http.Post(fmt.Sprintf("%s/api/qa", env.BaseURL), "application/json", bytes.NewBuffer(jsonData))
	require.NoError(t, err, "HTTP request should not fail")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "QA request should succeed")

	var response struct {
		Code int              `json:"code"`
		Data model.QAResponse `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err, "Failed to decode response")

	return response.Data
}

// deleteDocument 删除文档
func deleteDocument(t *testing.T, env *pythonIntegrationEnv, fileID string) {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/documents/%s", env.BaseURL, fileID), nil)
	require.NoError(t, err, "Failed to create request")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should not fail")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Delete should succeed")

	var response struct {
		Code int `json:"code"`
		Data struct {
			Success bool   `json:"success"`
			FileID  string `json:"file_id"`
		} `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err, "Failed to decode response")

	assert.True(t, response.Data.Success, "Delete operation should be successful")
	assert.Equal(t, fileID, response.Data.FileID, "Deleted file ID should match")
}
