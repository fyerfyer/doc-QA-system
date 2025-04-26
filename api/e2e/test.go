package e2e

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/mock"
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
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2EWorkflow 模拟端到端工作流测试
func TestE2EWorkflow(t *testing.T) {
	// 设置测试模式
	gin.SetMode(gin.TestMode)

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "docqa_e2e_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 1. 设置测试环境

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

	// 创建嵌入客户端 (如果有API密钥，使用真实客户端，否则使用Mock)
	var embeddingClient embedding.Client
	var llmClient llm.Client

	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey != "" {
		// 使用真实的通义千问客户端
		embeddingClient, err = embedding.NewClient("tongyi",
			embedding.WithAPIKey(apiKey),
			embedding.WithModel("text-embedding-v1"),
		)
		require.NoError(t, err)

		llmClient, err = llm.NewClient("tongyi",
			llm.WithAPIKey(apiKey),
			llm.WithModel("qwen-turbo"),
		)
		require.NoError(t, err)
	} else {
		// 使用Mock客户端
		mockEmbedding := embedding.NewMockClient(t)
		mockEmbedding.EXPECT().Name().Return("mock-embedding")
		mockEmbedding.EXPECT().Embed(mock.Anything, mock.Anything).Return(
			make([]float32, 1536), nil,
		)

		mockLLM := llm.NewMockClient(t)
		mockLLM.EXPECT().Name().Return("mock-llm")
		mockLLM.EXPECT().Generate(mock.Anything, mock.Anything, mock.Anything).Return(
			&llm.Response{
				Text:       "根据文档内容，这是关于向量数据库的文档。",
				TokenCount: 10,
				ModelName:  "mock-model",
				FinishTime: time.Now(),
			},
			nil,
		)

		embeddingClient = mockEmbedding
		llmClient = mockLLM
	}

	// 创建内存缓存
	cacheService, err := cache.NewCache(cache.Config{
		Type: "memory",
	})
	require.NoError(t, err)

	// 创建文本分段器
	splitter := document.NewTextSplitter(document.SplitterConfig{
		SplitType:    document.ByParagraph,
		ChunkSize:    1000,
		ChunkOverlap: 200,
	})

	// 创建RAG服务
	ragService := llm.NewRAG(llmClient,
		llm.WithRAGMaxTokens(256),
		llm.WithRAGTemperature(0.7),
	)

	// 创建文档服务
	documentService := services.NewDocumentService(
		fileStorage,
		nil, // 使用ParserFactory
		splitter,
		embeddingClient,
		vectorDB,
		services.WithBatchSize(5),
	)

	// 创建问答服务
	qaService := services.NewQAService(
		embeddingClient,
		vectorDB,
		llmClient,
		ragService,
		cacheService,
	)

	// 创建API处理器
	docHandler := handler.NewDocumentHandler(documentService, fileStorage)
	qaHandler := handler.NewQAHandler(qaService)

	// 设置路由
	router := api.SetupRouter(docHandler, qaHandler)

	// 2. 创建测试文件
	testFilePath := filepath.Join(tempDir, "test_doc.txt")
	testContent := "这是一个关于向量数据库的测试文档。向量数据库是专门用于高效存储和检索向量数据的数据库系统，常用于机器学习和人工智能应用场景。"
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	require.NoError(t, err)

	// 3. 上传文档
	file, err := os.Open(testFilePath)
	require.NoError(t, err)
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test_doc.txt")
	require.NoError(t, err)
	_, err = io.Copy(part, file)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/documents", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var uploadResp model.Response
	err = json.Unmarshal(w.Body.Bytes(), &uploadResp)
	require.NoError(t, err)

	uploadData := uploadResp.Data.(map[string]interface{})
	fileID := uploadData["file_id"].(string)
	require.NotEmpty(t, fileID)

	// 稍等片刻，确保异步处理完成
	time.Sleep(1 * time.Second)

	// 4. 发送问答请求
	qaBody := map[string]interface{}{
		"question": "什么是向量数据库?",
		"file_id":  fileID,
	}
	qaJSON, err := json.Marshal(qaBody)
	require.NoError(t, err)

	qaReq := httptest.NewRequest(http.MethodPost, "/api/qa", bytes.NewBuffer(qaJSON))
	qaReq.Header.Set("Content-Type", "application/json")
	qaW := httptest.NewRecorder()
	router.ServeHTTP(qaW, qaReq)

	assert.Equal(t, http.StatusOK, qaW.Code)

	var qaResp model.Response
	err = json.Unmarshal(qaW.Body.Bytes(), &qaResp)
	require.NoError(t, err)

	qaData := qaResp.Data.(map[string]interface{})
	answer := qaData["answer"].(string)
	t.Logf("问题回答: %s", answer)
	assert.NotEmpty(t, answer)

	// 5. 删除文档
	delReq := httptest.NewRequest(http.MethodDelete, "/api/documents/"+fileID, nil)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)

	assert.Equal(t, http.StatusOK, delW.Code)

	var delResp model.Response
	err = json.Unmarshal(delW.Body.Bytes(), &delResp)
	require.NoError(t, err)

	delData := delResp.Data.(map[string]interface{})
	success := delData["success"].(bool)
	assert.True(t, success)
}
