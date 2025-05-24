package pyprovider

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestDocumentClientIntegration 测试 DocumentClient 的集成
func TestDocumentClientIntegration(t *testing.T) {
    // 创建配置和客户端
    config := DefaultConfig()

    client, err := NewClient(config)
    require.NoError(t, err)
    require.NotNil(t, client)

    docClient := NewDocumentClient(client)
    require.NotNil(t, docClient)

    // 创建临时测试文件
    tempDir := t.TempDir()
    testFilePath := filepath.Join(tempDir, "test_document.txt")
    testContent := "这是一个测试文档。\n这是第二段落。"
    err = os.WriteFile(testFilePath, []byte(testContent), 0644)
    require.NoError(t, err)

    ctx := context.Background()

    // 测试使用文件路径解析
    t.Run("ParseDocument", func(t *testing.T) {
        result, err := docClient.ParseDocument(ctx, testFilePath)
        require.NoError(t, err)
        assert.NotNil(t, result)
        assert.Contains(t, result.Content, "这是一个测试文档")
    })

    // 测试使用Reader解析
    t.Run("ParseDocumentWithReader", func(t *testing.T) {
        reader := bytes.NewReader([]byte(testContent))
        result, err := docClient.ParseDocumentWithReader(ctx, reader, "test_document.txt")
        require.NoError(t, err)
        assert.NotNil(t, result)
        assert.Contains(t, result.Content, "这是一个测试文档")
    })

    // 测试获取已解析的文档内容
    t.Run("GetDocumentContent", func(t *testing.T) {
        // 先解析文档以确保有存储结果
        result, err := docClient.ParseDocument(ctx, testFilePath)
        require.NoError(t, err)
        
        // 从解析结果中获取文档ID
        documentID := result.DocumentID
        
        retrievedResult, err := docClient.GetDocumentContent(ctx, documentID)
        if err != nil {
            t.Errorf("获取文档内容失败: %v", err)
        }
        
        assert.NotNil(t, retrievedResult)
        assert.Contains(t, retrievedResult.Content, "这是一个测试文档")
    })

    // 测试错误情况：文件不存在
    t.Run("FileNotFound", func(t *testing.T) {
        _, err := docClient.ParseDocument(ctx, "non_existent_file.txt")
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "File not found") // 或类似的错误消息
    })

    // 测试空文件
    t.Run("EmptyFile", func(t *testing.T) {
        emptyFilePath := filepath.Join(tempDir, "empty.txt")
        err = os.WriteFile(emptyFilePath, []byte{}, 0644)
        require.NoError(t, err)

        result, err := docClient.ParseDocument(ctx, emptyFilePath)
		assert.NoError(t, err) // 应该不报错
		assert.NotNil(t, result) // 这里先检查是否为nil
		if result != nil { 
			assert.Empty(t, result.Content) // 内容应为空
		}
    })

    // 测试非文本文件
    t.Run("BinaryFile", func(t *testing.T) {
        // 创建一个二进制文件
        binaryFilePath := filepath.Join(tempDir, "binary.bin")
        binaryData := []byte{0x00, 0x01, 0x02, 0x03}
        err = os.WriteFile(binaryFilePath, binaryData, 0644)
        require.NoError(t, err)

        // Python服务应该尝试解析，可能返回乱码或错误
        result, err := docClient.ParseDocument(ctx, binaryFilePath)
        if err != nil {
            // 如果报错了，那么错误消息应该有意义
            assert.Contains(t, strings.ToLower(err.Error()), "document parsing failed") // "解析失败"或类似消息
        } else {
            // 如果没报错，结果不应为nil
            assert.NotNil(t, result)
        }
    })
}