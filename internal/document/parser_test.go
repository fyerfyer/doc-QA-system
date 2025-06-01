package document

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/fyerfyer/doc-QA-system/internal/pyprovider"
    "github.com/fyerfyer/doc-QA-system/pkg/storage"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// 创建测试用例存储文件系统
func setupMinioTestStorage(t *testing.T) storage.Storage {
    t.Helper()

    // 使用Minio存储
    config := storage.MinioConfig{
        Endpoint:  "localhost:9000",
        AccessKey: "minioadmin", // 默认的MinIO访问密钥
        SecretKey: "minioadmin", // 默认的MinIO密钥
        Bucket:    "docqa",
        UseSSL:    false,
    }

    // 创建MinIO存储客户端
    s, err := storage.NewMinioStorage(config)
    require.NoError(t, err, "Failed to create MinIO storage")
    
    return s
}

// 创建Python解析器
func setupPythonParser(t *testing.T) Parser {
    t.Helper()

    // 创建Python服务配置
    config := pyprovider.DefaultConfig()
    // 设置超时，防止测试长时间阻塞
    config.Timeout = 30 * time.Second
    
    // 创建HTTP客户端
    httpClient, err := pyprovider.NewClient(config)
    require.NoError(t, err, "Failed to create Python HTTP client")

    // 创建文档客户端
    docClient := pyprovider.NewDocumentClient(httpClient)

    // 创建Python解析器
    return NewPythonParser(docClient)
}

// 创建带上下文的Python解析器
func setupPythonAwareParser(t *testing.T) PythonAwareParser {
    t.Helper()

    // 创建Python服务配置
    config := pyprovider.DefaultConfig()
    // 设置超时，防止测试长时间阻塞
    config.Timeout = 30 * time.Second
    
    // 创建HTTP客户端
    httpClient, err := pyprovider.NewClient(config)
    require.NoError(t, err, "Failed to create Python HTTP client")

    // 创建文档客户端
    docClient := pyprovider.NewDocumentClient(httpClient)

    // 创建Python解析器
    return NewPythonAwareParser(docClient)
}

// 测试创建上传文件并返回路径
func uploadTestFile(t *testing.T, s storage.Storage, filename string, content []byte) string {
    t.Helper()

    reader := bytes.NewReader(content)
    fileInfo, err := s.Save(reader, filename)
    require.NoError(t, err, "Failed to save test file")
    
    return fileInfo.Path
}

// 测试Python解析器的Parse方法
func TestPythonParserParse(t *testing.T) {
    // 设置存储和解析器
    s := setupMinioTestStorage(t)
    parser := setupPythonParser(t)
    
    // 创建测试文件
    content := []byte("# Test Markdown\n\nThis is a test document for Python parser.")
    filePath := uploadTestFile(t, s, "test.md", content)
    
    // 测试解析
    parsedContent, err := parser.Parse(filePath)
    
    // 验证结果
    assert.NoError(t, err, "Parse should not return error")
    assert.Contains(t, parsedContent, "Test Markdown", "Parsed content should contain the header")
    assert.Contains(t, parsedContent, "test document", "Parsed content should contain the body text")
}

// 测试Python解析器的ParseReader方法
func TestPythonParserParseReader(t *testing.T) {
    // 设置解析器
    parser := setupPythonParser(t)
    
    // 创建测试内容
    content := "# Test Reader Content\n\nThis is a test for ParseReader method."
    reader := bytes.NewBufferString(content)
    
    // 测试解析
    parsedContent, err := parser.ParseReader(reader, "test.md")
    
    // 验证结果
    assert.NoError(t, err, "ParseReader should not return error")
    assert.Contains(t, parsedContent, "Test Reader Content", "Parsed content should contain the header")
    assert.Contains(t, parsedContent, "test for ParseReader", "Parsed content should contain the body text")
}

// 测试带上下文的Python解析器
func TestPythonAwareParserWithContext(t *testing.T) {
    // 设置存储和解析器
    s := setupMinioTestStorage(t)
    parser := setupPythonAwareParser(t)
    
    // 创建测试文件
    content := []byte("# Context Test\n\nTesting ParseWithContext method.")
    filePath := uploadTestFile(t, s, "context_test.md", content)
    
    // 创建带有超时的上下文
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // 测试解析
    parsedContent, err := parser.ParseWithContext(ctx, filePath)
    
    // 验证结果
    assert.NoError(t, err, "ParseWithContext should not return error")
    assert.Contains(t, parsedContent, "Context Test", "Parsed content should contain the header")
    assert.Contains(t, parsedContent, "Testing ParseWithContext", "Parsed content should contain the body text")
}

// 测试带上下文的Python解析器的Reader方法
func TestPythonAwareParserReaderWithContext(t *testing.T) {
    // 设置解析器
    parser := setupPythonAwareParser(t)
    
    // 创建测试内容
    content := "# Reader Context Test\n\nTesting ParseReaderWithContext method."
    reader := bytes.NewBufferString(content)
    
    // 创建带有超时的上下文
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // 测试解析
    parsedContent, err := parser.ParseReaderWithContext(ctx, reader, "test_context.md")
    
    // 验证结果
    assert.NoError(t, err, "ParseReaderWithContext should not return error")
    assert.Contains(t, parsedContent, "Reader Context Test", "Parsed content should contain the header")
    assert.Contains(t, parsedContent, "Testing ParseReaderWithContext", "Parsed content should contain the body text")
}

// 测试Python解析器工厂函数
func TestPythonParserFactory(t *testing.T) {
    // 测试默认创建方式
    parser, err := PythonParserFactory(nil)
    assert.NoError(t, err, "Factory should create parser with default client")
    assert.NotNil(t, parser, "Created parser should not be nil")
    
    // 创建自定义客户端并测试
    config := pyprovider.DefaultConfig()
    httpClient, err := pyprovider.NewClient(config)
    require.NoError(t, err, "Failed to create Python HTTP client")
    docClient := pyprovider.NewDocumentClient(httpClient)
    
    parser, err = PythonParserFactory(docClient)
    assert.NoError(t, err, "Factory should create parser with custom client")
    assert.NotNil(t, parser, "Created parser should not be nil")
    
    // 测试错误情况 - 无效的客户端类型
    _, err = PythonParserFactory("invalid client")
    assert.Error(t, err, "Factory should return error for invalid client type")
}

// 测试解析PDF文件
func TestPythonParserParsePDF(t *testing.T) {
    // 确认测试PDF文件存在
    pdfPath := filepath.Join("testdata", "sample.pdf")
    if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
        t.Skip("Sample PDF file not found in testdata directory")
    }

    // 读取PDF文件
    pdfFile, err := os.Open(pdfPath)
    require.NoError(t, err, "Failed to open sample PDF file")
    defer pdfFile.Close()
    
    // 设置存储和解析器
    s := setupMinioTestStorage(t)
    parser := setupPythonParser(t)
    
    // 上传PDF文件到存储
    fileInfo, err := s.Save(pdfFile, "sample.pdf")
    require.NoError(t, err, "Failed to save PDF file")
    
    // 测试解析
    parsedContent, err := parser.Parse(fileInfo.Path)
    
    // 验证结果
    assert.NoError(t, err, "Parse should not return error for PDF")
    assert.NotEmpty(t, parsedContent, "Parsed PDF content should not be empty")
}

// 测试大文件解析
func TestPythonParserLargeFile(t *testing.T) {
    // 设置存储和解析器
    s := setupMinioTestStorage(t)
    parser := setupPythonParser(t)
    
    // 创建一个大文本文件（约1MB）
    var largeContent bytes.Buffer
    lineTemplate := "This is line %d of the large test file for Python parser testing.\n"
    for i := 0; i < 20000; i++ {
        largeContent.WriteString(lineTemplate + lineTemplate) // 约100字节 × 2 × 20000 = ~4MB
    }
    
    // 上传大文件
    filePath := uploadTestFile(t, s, "large_file.txt", largeContent.Bytes())
    
    // 测试解析
    parsedContent, err := parser.Parse(filePath)
    
    // 验证结果
    assert.NoError(t, err, "Parse should handle large files without error")
    assert.NotEmpty(t, parsedContent, "Parsed content should not be empty")
    assert.Contains(t, parsedContent, "This is line", "Parsed content should contain original text")
}

// 测试无效的客户端
func TestPythonParserWithNilClient(t *testing.T) {
    // 创建一个客户端为nil的解析器
    parser := &PythonParser{client: nil}
    
    // 测试Parse方法
    _, err := parser.Parse("test.txt")
    assert.Error(t, err, "Parse should return error with nil client")
    assert.Contains(t, err.Error(), "uninitialized", "Error should mention uninitialized client")
    
    // 测试ParseReader方法
    _, err = parser.ParseReader(bytes.NewBufferString("test"), "test.txt")
    assert.Error(t, err, "ParseReader should return error with nil client")
    assert.Contains(t, err.Error(), "uninitialized", "Error should mention uninitialized client")
}

// 测试错误的文件路径
func TestPythonParserInvalidPath(t *testing.T) {
    // 设置解析器
    parser := setupPythonParser(t)
    
    // 测试不存在的文件路径
    _, err := parser.Parse("not_exist_file.txt")
    assert.Error(t, err, "Parse should return error for non-existent file")
}