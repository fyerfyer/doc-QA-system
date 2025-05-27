package document

import (
    "testing"
    "time"

    "github.com/fyerfyer/doc-QA-system/internal/pyprovider"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// 创建测试用的Python分块器
func setupPythonSplitter(t *testing.T) (*PythonSplitter, error) {
    t.Helper()

    // 创建Python服务配置
    config := pyprovider.DefaultConfig()
    // 设置超时，防止测试长时间阻塞
    config.Timeout = 30 * time.Second
    
    // 创建HTTP客户端
    httpClient, err := pyprovider.NewClient(config)
    if err != nil {
        return nil, err
    }

    // 创建文档客户端
    docClient := pyprovider.NewDocumentClient(httpClient)

    // 创建默认配置
    splitConfig := DefaultSplitterConfig()
    
    // 创建并返回分块器
    return &PythonSplitter{
        client:     docClient,
        chunkSize:  splitConfig.ChunkSize,
        overlap:    splitConfig.Overlap,
        splitType:  splitConfig.SplitType,
        documentID: splitConfig.DocumentID,
    }, nil
}

// 测试创建分块器
func TestNewPythonSplitter(t *testing.T) {
    // 创建Python服务配置和客户端
    config := pyprovider.DefaultConfig()
    httpClient, err := pyprovider.NewClient(config)
    require.NoError(t, err, "Failed to create Python HTTP client")
    docClient := pyprovider.NewDocumentClient(httpClient)
    
    // 创建分块配置
    splitConfig := SplitConfig{
        ChunkSize:  500,
        Overlap:    100,
        SplitType:  "sentence",
        DocumentID: "test-doc-123",
    }
    
    // 创建分块器
    splitter := NewPythonSplitter(docClient, splitConfig)
    
    // 验证分块器是否正确创建
    pythonSplitter, ok := splitter.(*PythonSplitter)
    assert.True(t, ok, "Created splitter should be PythonSplitter type")
    assert.Equal(t, 500, pythonSplitter.GetChunkSize(), "Chunk size should match configuration")
    assert.Equal(t, 100, pythonSplitter.GetOverlap(), "Overlap should match configuration")
    assert.Equal(t, "sentence", pythonSplitter.GetSplitType(), "Split type should match configuration")
}

// 测试分块器工厂函数
func TestNewTextSplitter(t *testing.T) {
    // 创建自定义配置
    cfg := SplitConfig{
        ChunkSize:  800,
        Overlap:    150,
        SplitType:  "sentence",
        DocumentID: "test-doc-456",
    }
    
    // 创建分块器
    splitter, err := NewTextSplitter(cfg)
    assert.NoError(t, err, "NewTextSplitter should not return error")
    assert.NotNil(t, splitter, "Created splitter should not be nil")
    
    // 验证分块器类型和配置
    pythonSplitter, ok := splitter.(*PythonSplitter)
    assert.True(t, ok, "Created splitter should be PythonSplitter type")
    assert.Equal(t, 800, pythonSplitter.GetChunkSize(), "Chunk size should match configuration")
    assert.Equal(t, 150, pythonSplitter.GetOverlap(), "Overlap should match configuration")
    assert.Equal(t, "sentence", pythonSplitter.GetSplitType(), "Split type should match configuration")
}

// 测试分块操作 - 句子分块
func TestPythonSplitterSplitSentence(t *testing.T) {
    // 创建Python服务配置和客户端
    config := pyprovider.DefaultConfig()
    httpClient, err := pyprovider.NewClient(config)
    require.NoError(t, err, "Failed to create Python HTTP client")
    docClient := pyprovider.NewDocumentClient(httpClient)
    
    // 创建句子分块配置
    splitConfig := SplitConfig{
        ChunkSize:  200,
        Overlap:    50,
        SplitType:  "sentence", // 使用句子分块
        DocumentID: "test-sentence",
    }
    
    // 创建分块器
    splitter := NewPythonSplitter(docClient, splitConfig)
    
    // 准备测试文本（多个句子）
    text := "这是第一个句子。这是第二个句子，它比较长一些。这是第三个句子！这是第四个句子？这是最后一个句子。"
    
    // 执行分块
    contents, err := splitter.Split(text)
    
    // 验证结果
    assert.NoError(t, err, "Split should not return error")
    assert.NotEmpty(t, contents, "Should return non-empty content list")
    
    // 句子分块可能会将紧密相连的多个句子合并到一个块中，具体取决于Python服务的实现
    // 这里我们只验证基本功能是否正常
    for i, content := range contents {
        assert.NotEmpty(t, content.Text, "Chunk %d text should not be empty", i)
        assert.Equal(t, i, content.Index, "Chunk %d should have index %d", i, i)
    }
}

// 测试空文本分块
func TestPythonSplitterEmptyText(t *testing.T) {
    // 创建分块器
    splitter, err := setupPythonSplitter(t)
    require.NoError(t, err, "Failed to create Python splitter")
    
    // 执行空文本分块
    _, err = splitter.Split("")
    
    // 验证结果 - 应该不返回错误，但结果为空
    assert.Error(t, err, "Split should return error for empty text")
}

// 测试无效的客户端
func TestPythonSplitterWithNilClient(t *testing.T) {
    // 创建一个客户端为nil的分块器
    splitter := &PythonSplitter{
        client:     nil,
        chunkSize:  1000,
        overlap:    200,
        splitType:  "paragraph",
        documentID: "test-nil-client",
    }
    
    // 执行分块
    _, err := splitter.Split("测试文本")
    
    // 验证结果
    assert.Error(t, err, "Split should return error with nil client")
    assert.Contains(t, err.Error(), "uninitialized", "Error should mention uninitialized client")
}

// 测试大文本分块
func TestPythonSplitterLargeText(t *testing.T) {
    // 创建分块器
    splitter, err := setupPythonSplitter(t)
    require.NoError(t, err, "Failed to create Python splitter")
    
    // 创建一个大文本（约50KB）
    var largeText string
    line := "这是测试大文本分块功能的一行。我们需要创建足够大的文本来测试Python分块器的性能和稳定性。"
    for i := 0; i < 500; i++ {
        largeText += line + "\n"
    }
    
    // 执行分块
    contents, err := splitter.Split(largeText)
    
    // 验证结果
    assert.NoError(t, err, "Split should not return error for large text")
    assert.NotEmpty(t, contents, "Should return non-empty content list")
    assert.GreaterOrEqual(t, len(contents), 10, "Should return many chunks for large text")
}

// 测试获取器方法
func TestPythonSplitterGetters(t *testing.T) {
    // 创建分块器
    splitter := PythonSplitter{
        chunkSize:  500,
        overlap:    100,
        splitType:  "sentence",
        documentID: "test-getters",
    }
    
    // 验证获取器方法
    assert.Equal(t, 500, splitter.GetChunkSize(), "GetChunkSize should return correct value")
    assert.Equal(t, 100, splitter.GetOverlap(), "GetOverlap should return correct value")
    assert.Equal(t, "sentence", splitter.GetSplitType(), "GetSplitType should return correct value")
}