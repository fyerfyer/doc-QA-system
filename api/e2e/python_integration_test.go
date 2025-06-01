package e2e

import (
    "bytes"
    "context"
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
        t.Fatal("DASHSCOPE_API_KEY environment variable is required for tests")
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
        os.Remove(dbPath)
    })

    // 初始化MinIO存储 - 使用Docker容器地址
    minioStorage, err := storage.NewMinioStorage(storage.MinioConfig{
        Endpoint:  "172.18.0.2:9000", // 使用Docker容器地址
        AccessKey: "minioadmin",
        SecretKey: "minioadmin",
        UseSSL:    false,
        Bucket:    "docqa-test",
    })
    require.NoError(t, err, "Failed to create MinIO storage")

    env.Storage = minioStorage

    // 设置Redis缓存 - 使用Docker容器地址
    redisCache, err := cache.NewCache(cache.Config{
        Type:       "redis",
        RedisAddr:  "172.18.0.3:6379", // 使用Docker容器地址
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
        os.RemoveAll(faissIndexPath)
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

    // 创建任务队列 - 使用Docker容器地址
    queueConfig := taskqueue.DefaultConfig()
    queueConfig.RedisAddr = "172.18.0.3:6379" // 使用Docker容器地址
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

func TestPythonWorkerConnectivity(t *testing.T) {
    // Try to access Python worker's health endpoint using Docker container address
    urls := []string{
        "http://172.18.0.5:8000/api/health/ping", // 使用Docker容器地址
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    client := &http.Client{Timeout: 3 * time.Second}

    for _, url := range urls {
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
        require.NoError(t, err, "Failed to create request")

        resp, err := client.Do(req)
        if err != nil {
            t.Logf("Error connecting to %s: %v", url, err)
            continue
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        require.NoError(t, err, "Failed to read response body")

        t.Logf("Response from %s: Status=%d, Body=%s", url, resp.StatusCode, string(body))
        
        // Verify the response is as expected
        assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200 OK")
        
        // Check if response contains the expected "ping":"pong" response
        var pingResponse map[string]string
        err = json.Unmarshal(body, &pingResponse)
        require.NoError(t, err, "Failed to parse ping response")
        assert.Equal(t, "pong", pingResponse["ping"], "Expected ping:pong response")
        
        t.Logf("Successfully connected to Python worker health endpoint")
    }

    t.Log("Testing Redis connectivity...")
    // Create Redis client with Docker container address
    redisConfig := cache.Config{
        Type:      "redis",
        RedisAddr: "172.18.0.3:6379", // 使用Docker容器地址
    }
    
    redisCache, err := cache.NewCache(redisConfig)
    if err != nil {
        t.Logf("Failed to connect to Redis: %v", err)
    } else {
        // Test Redis with a simple key-value operation
        err = redisCache.Set("test-key", "test-value", 10*time.Second)
        if err != nil {
            t.Errorf("Failed to set value in Redis: %v", err)
        }
        
        val, found, err := redisCache.Get("test-key")
        if err != nil || !found || val != "test-value" {
            t.Errorf("Redis test failed: err=%v, found=%v, val=%s", err, found, val)
        } else {
            t.Logf("Redis connectivity test passed")
        }
    }
}

// TestPythonIntegration 测试Python服务集成
func TestPythonIntegration(t *testing.T) {
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
    require.NotEmpty(t, fileID, "Expected file ID to be non-empty")

    // 第2步：等待文档处理完成
    t.Log("Step 2: Waiting for document processing")
    waitForDocumentProcessing(t, env, fileID)

    // 第3步：检查文档状态
    t.Log("Step 3: Checking document status")
    status, err := getDocumentStatus(t, env, fileID)
    require.NoError(t, err, "Failed to get document status")
    t.Logf("Document status: %+v", status)

    // 第4步：发送问题查询
    t.Log("Step 4: Testing question answering")
    question := "这个文档的主要功能是什么？"
    answer := askQuestion(t, env, question, fileID)
    t.Logf("Question: %s\nAnswer: %s", question, answer.Answer)
    require.NotEmpty(t, answer.Answer, "Expected non-empty answer")

    // 第5步：测试不指定文档ID的通用问答
    t.Log("Step 5: Testing general QA without document ID")
    generalQuestion := "文本分块的作用是什么？"
    generalAnswer := askQuestion(t, env, generalQuestion, "")
    t.Logf("General Question: %s\nGeneral Answer: %s", generalQuestion, generalAnswer.Answer)

    // 第6步：删除文档
    t.Log("Step 6: Testing document deletion")
    deleteDocument(t, env, fileID)

    // 检查文档是否已删除
    _, err = getDocumentStatus(t, env, fileID)
    require.Error(t, err, "Expected error when getting deleted document status")
}

// TestPythonServiceError 测试Python服务错误处理
func TestPythonServiceError(t *testing.T) {
    // 设置测试环境
    env := setupPythonTestEnv(t)
    defer env.cleanup()

    // 测试上传不支持的文件类型
    t.Log("Testing unsupported file type")

    // 创建一个不支持的文件类型
    unsupportedFile := createPyTestFile(t, env.TempDir, "test.xyz", "测试内容")

    // 上传不支持的文件
    body := new(bytes.Buffer)
    writer := multipart.NewWriter(body)
    
    // 添加文件
    file, err := os.Open(unsupportedFile)
    require.NoError(t, err, "Failed to open test file")
    defer file.Close()
    
    part, err := writer.CreateFormFile("file", "test.xyz")
    require.NoError(t, err, "Failed to create form file")
    
    _, err = io.Copy(part, file)
    require.NoError(t, err, "Failed to copy file content")

    // 完成multipart写入
    err = writer.Close()
    require.NoError(t, err, "Failed to close multipart writer")

    // 发送请求
    req, err := http.NewRequest("POST", env.BaseURL+"/api/documents", body)
    require.NoError(t, err, "Failed to create request")
    req.Header.Set("Content-Type", writer.FormDataContentType())

    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err, "Failed to send request")
    defer resp.Body.Close()

    // 验证响应状态码（应该是400 Bad Request，因为文件类型不支持）
    require.Equal(t, http.StatusBadRequest, resp.StatusCode, "Expected status code 400 for unsupported file type")
}

// uploadDocument 上传文档并返回文档ID
func uploadDocument(t *testing.T, env *pythonIntegrationEnv, filePath string, fileName string) string {
    t.Logf("Uploading document: %s", fileName)

    body := new(bytes.Buffer)
    writer := multipart.NewWriter(body)

    // 添加文件
    file, err := os.Open(filePath)
    require.NoError(t, err, "Failed to open test file")
    defer file.Close()

    part, err := writer.CreateFormFile("file", fileName)
    require.NoError(t, err, "Failed to create form file")

    _, err = io.Copy(part, file)
    require.NoError(t, err, "Failed to copy file content")

    // 添加标签
    err = writer.WriteField("tags", "test,integration")
    require.NoError(t, err, "Failed to add tags field")

    // 完成multipart写入
    err = writer.Close()
    require.NoError(t, err, "Failed to close multipart writer")

    // 发送请求
    req, err := http.NewRequest("POST", env.BaseURL+"/api/documents", body)
    require.NoError(t, err, "Failed to create request")
    req.Header.Set("Content-Type", writer.FormDataContentType())

    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err, "Failed to send request")
    defer resp.Body.Close()

    require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200")

    respBody, err := io.ReadAll(resp.Body)
    require.NoError(t, err, "Failed to read response body")

    var response model.Response
    err = json.Unmarshal(respBody, &response)
    require.NoError(t, err, "Failed to parse JSON response")

    var uploadResp model.DocumentUploadResponse
    b, err := json.Marshal(response.Data)
    require.NoError(t, err, "Failed to re-marshal data")

    err = json.Unmarshal(b, &uploadResp)
    require.NoError(t, err, "Failed to parse upload response")

    t.Logf("Document uploaded successfully, file ID: %s", uploadResp.FileID)
    return uploadResp.FileID
}

// waitForDocumentProcessing 等待文档处理完成
func waitForDocumentProcessing(t *testing.T, env *pythonIntegrationEnv, fileID string) {
    // 等待最长2分钟
    timeout := time.Now().Add(2 * time.Minute)

    for time.Now().Before(timeout) {
        status, err := getDocumentStatus(t, env, fileID)
        if err != nil {
            t.Logf("Error checking document status: %v", err)
            time.Sleep(2 * time.Second)
            continue
        }

        t.Logf("Document status: %s, progress: %d%%", status.Status, status.Progress)

        if status.Status == "completed" {
            t.Log("Document processing completed successfully")
            return
        } else if status.Status == "failed" {
            t.Fatalf("Document processing failed: %s", status.Error)
        }

        time.Sleep(5 * time.Second)
    }

    t.Fatal("Document processing timed out")
}

// getDocumentStatus 获取文档状态
func getDocumentStatus(t *testing.T, env *pythonIntegrationEnv, fileID string) (*model.DocumentStatusResponse, error) {
    resp, err := http.Get(fmt.Sprintf("%s/api/documents/%s/status", env.BaseURL, fileID))
    if err != nil {
        return nil, fmt.Errorf("failed to request status: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response body: %w", err)
    }

    var response model.Response
    if err := json.Unmarshal(body, &response); err != nil {
        return nil, fmt.Errorf("failed to parse JSON response: %w", err)
    }

    var status model.DocumentStatusResponse
    b, err := json.Marshal(response.Data)
    if err != nil {
        return nil, fmt.Errorf("failed to re-marshal data: %w", err)
    }

    if err := json.Unmarshal(b, &status); err != nil {
        return nil, fmt.Errorf("failed to parse status data: %w", err)
    }

    return &status, nil
}

// askQuestion 发送问题查询
func askQuestion(t *testing.T, env *pythonIntegrationEnv, question string, fileID string) model.QAResponse {
    reqBody := map[string]interface{}{
        "question": question,
    }
    if fileID != "" {
        reqBody["file_id"] = fileID
    }

    jsonBody, err := json.Marshal(reqBody)
    require.NoError(t, err, "Failed to marshal request body")

    resp, err := http.Post(env.BaseURL+"/api/qa", "application/json", bytes.NewBuffer(jsonBody))
    require.NoError(t, err, "Failed to send question")
    defer resp.Body.Close()

    t.Logf("QA response status code: %d", resp.StatusCode)
    require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200")

    body, err := io.ReadAll(resp.Body)
    require.NoError(t, err, "Failed to read response body")

    var response model.Response
    err = json.Unmarshal(body, &response)
    require.NoError(t, err, "Failed to parse JSON response")

    var qaResp model.QAResponse
    b, err := json.Marshal(response.Data)
    require.NoError(t, err, "Failed to re-marshal data")

    err = json.Unmarshal(b, &qaResp)
    require.NoError(t, err, "Failed to parse QA response")

    return qaResp
}

// deleteDocument 删除文档
func deleteDocument(t *testing.T, env *pythonIntegrationEnv, fileID string) {
    req, err := http.NewRequest("DELETE", env.BaseURL+"/api/documents/"+fileID, nil)
    require.NoError(t, err, "Failed to create request")

    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err, "Failed to send request")
    defer resp.Body.Close()

    require.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200")

    body, err := io.ReadAll(resp.Body)
    require.NoError(t, err, "Failed to read response body")

    var response model.Response
    err = json.Unmarshal(body, &response)
    require.NoError(t, err, "Failed to parse JSON response")

    t.Log("Document deleted successfully")
}