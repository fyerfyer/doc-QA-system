package services

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
)

// TestDocumentService 测试文档服务的基本功能
func TestDocumentService(t *testing.T) {
	// 创建临时文件和目录
	tempDir, err := ioutil.TempDir("", "docqa-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 创建测试文件
	testContent := "这是一个测试文档内容。\n\n这是第二段落。\n\n这是第三段落。"
	testFile := filepath.Join(tempDir, "test.txt")
	err = ioutil.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// 初始化测试环境和服务
	docService, vectorDB := setupDocumentTestEnv(t, tempDir)

	// 测试文档处理流程
	ctx := context.Background()
	fileID := "test-file-id"
	err = docService.ProcessDocument(ctx, fileID, testFile)
	require.NoError(t, err, "Document processing should succeed")

	// 验证文档段落是否正确存入向量库
	filter := vectordb.SearchFilter{
		FileIDs:    []string{fileID},
		MaxResults: 10,
	}
	queryVector := make([]float32, 4) // 创建空向量用于查询
	results, err := vectorDB.Search(queryVector, filter)
	require.NoError(t, err)
	assert.Equal(t, 3, len(results), "There should be 3 paragraphs saved")

	// 测试获取文档信息
	docInfo, err := docService.GetDocumentInfo(ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, fileID, docInfo["file_id"])
	assert.Equal(t, DocStatusCompleted, docInfo["status"])

	// 测试删除文档
	err = docService.DeleteDocument(ctx, fileID)
	require.NoError(t, err, "Document deletion should succeed")

	// 确认段落已被删除
	results, err = vectorDB.Search(queryVector, filter)
	require.NoError(t, err)
	assert.Empty(t, results, "There should be no paragraphs after document deletion")
}

// TestProcessDocumentWithDifferentTypes 测试处理不同类型的文档
func TestProcessDocumentWithDifferentTypes(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "docqa-multitype-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 创建各种测试文件
	testFiles := map[string]string{
		"text.txt": "纯文本测试内容",
		"doc.md":   "# 标题\n\n这是**Markdown**文件",
	}

	createdFiles := make(map[string]string)
	for name, content := range testFiles {
		filePath := filepath.Join(tempDir, name)
		err = ioutil.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)
		createdFiles[name] = filePath
	}

	// 初始化服务
	docService, vectorDB := setupDocumentTestEnv(t, tempDir)
	ctx := context.Background()

	// 测试处理不同类型的文件
	for name, path := range createdFiles {
		fileID := "file-" + name
		err = docService.ProcessDocument(ctx, fileID, path)
		require.NoError(t, err, "Processing %s should succeed", name)

		// 验证向量库中是否存在该文件的段落
		filter := vectordb.SearchFilter{
			FileIDs:    []string{fileID},
			MaxResults: 10,
		}
		queryVector := make([]float32, 4)
		results, err := vectorDB.Search(queryVector, filter)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find paragraphs for file %s", name)
	}
}

// 设置文档测试环境
func setupDocumentTestEnv(t *testing.T, tempDir string) (*DocumentService, vectordb.Repository) {
	// 创建存储服务
	storageConfig := storage.LocalConfig{
		Path: tempDir,
	}
	storageService, err := storage.NewLocalStorage(storageConfig)
	require.NoError(t, err)

	// 创建文本分段器
	splitterConfig := document.DefaultSplitterConfig()
	splitterConfig.ChunkSize = 100 // 测试用小段落
	textSplitter := document.NewTextSplitter(splitterConfig)

	// 创建嵌入客户端 - 使用已有的mock
	embeddingClient := &testEmbeddingClient{dimension: 4}

	// 创建向量数据库 - 使用内存版以简化测试
	vectorDBConfig := vectordb.Config{
		Type:      "memory",
		Dimension: 4,
	}
	vectorDB, err := vectordb.NewRepository(vectorDBConfig)
	require.NoError(t, err)

	// 创建文档服务
	docService := NewDocumentService(
		storageService,
		&testParser{},
		textSplitter,
		embeddingClient,
		vectorDB,
		WithBatchSize(2), // 小批量以便测试
		WithTimeout(5*time.Second),
	)

	return docService, vectorDB
}

// testParser 用于测试的简单解析器
type testParser struct{}

func (p *testParser) Parse(filePath string) (string, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// testEmbeddingClient 用于测试的简单嵌入客户端
type testEmbeddingClient struct {
	dimension int
}

func (c *testEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	// 返回固定维度的向量
	return generateTestVector(c.dimension, text), nil
}

func (c *testEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// 为每个文本生成一个向量
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = generateTestVector(c.dimension, text)
	}
	return vectors, nil
}

func (c *testEmbeddingClient) Name() string {
	return "test-embedding"
}

// generateTestVector 生成用于测试的向量
// 使用text的第一个字符作为种子以生成具有一定相似度的向量
func generateTestVector(dim int, text string) []float32 {
	vec := make([]float32, dim)
	seed := 0.1 // 默认种子
	if len(text) > 0 {
		// 使用第一个字符作为种子
		seed = float64(text[0]) / 255.0
	}

	for i := range vec {
		vec[i] = float32(seed + float64(i)*0.1)
	}
	return vec
}
