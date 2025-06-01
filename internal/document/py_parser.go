package document

import (
    "context"
    "errors"
    "fmt"
    "io"

    "github.com/fyerfyer/doc-QA-system/internal/pyprovider"
)

// PythonParser 使用Python服务的文档解析器实现
type PythonParser struct {
    client *pyprovider.DocumentClient // Python文档客户端
}

// NewPythonParser 创建一个新的Python解析器
func NewPythonParser(client *pyprovider.DocumentClient) Parser {
    return &PythonParser{client: client}
}

// Parse 通过Python服务解析文档
func (p *PythonParser) Parse(filePath string) (string, error) {
    if p.client == nil {
        return "", errors.New("python client uninitialized")
    }

    // 调用Python服务解析文档
    ctx := context.Background()
    result, err := p.client.ParseDocument(ctx, filePath)
    if err != nil {
        // TODO: 实现回退机制，在Python解析失败时尝试本地解析
        return "", fmt.Errorf("failed to parse document by python: %w", err)
    }

    return result.Content, nil
}

// ParseReader 通过Python服务从Reader解析文档
func (p *PythonParser) ParseReader(r io.Reader, filename string) (string, error) {
    if p.client == nil {
        return "", errors.New("python client uninitialized")
    }

    // 调用Python服务解析文档
    ctx := context.Background()
    result, err := p.client.ParseDocumentWithReader(ctx, r, filename)
    if err != nil {
        // TODO: 实现回退机制，在Python解析失败时尝试本地解析
        return "", fmt.Errorf("failed to parse document by python: %w", err)
    }

    return result.Content, nil
}

// PythonAwareParserImpl 支持Python API调用的增强解析器实现
type PythonAwareParserImpl struct {
    PythonParser // 嵌入基本的Python解析器
}

// NewPythonAwareParser 创建支持Python API调用的增强解析器
func NewPythonAwareParser(client *pyprovider.DocumentClient) PythonAwareParser {
    return &PythonAwareParserImpl{
        PythonParser: PythonParser{client: client},
    }
}

// ParseWithContext 使用上下文通过Python服务解析文档
func (p *PythonAwareParserImpl) ParseWithContext(ctx context.Context, filePath string) (string, error) {
    if p.client == nil {
        return "", errors.New("python client uninitialized")
    }

    // 调用Python服务解析文档
    result, err := p.client.ParseDocument(ctx, filePath)
    if err != nil {
        // TODO: 实现回退机制，在Python解析失败时尝试本地解析
        return "", fmt.Errorf("failed to parse document by python: %w", err)
    }

    return result.Content, nil
}

// ParseReaderWithContext 使用上下文从Reader通过Python服务解析文档
func (p *PythonAwareParserImpl) ParseReaderWithContext(ctx context.Context, r io.Reader, filename string) (string, error) {
    if p.client == nil {
        return "", errors.New("python client uninitialized")
    }

    // 调用Python服务解析文档
    result, err := p.client.ParseDocumentWithReader(ctx, r, filename)
    if err != nil {
        // TODO: 实现回退机制，在Python解析失败时尝试本地解析
        return "", fmt.Errorf("failed to parse document by python: %w", err)
    }

    return result.Content, nil
}

// PythonParserFactory 创建Python解析器的工厂函数
func PythonParserFactory(pythonClient interface{}) (PythonAwareParser, error) {
    // 检查传入的客户端类型
    if pythonClient == nil {
        // 尝试创建默认客户端
        config := pyprovider.DefaultConfig()
        httpClient, err := pyprovider.NewClient(config)
        if err != nil {
            return nil, fmt.Errorf("failed to create default python client: %w", err)
        }
        docClient := pyprovider.NewDocumentClient(httpClient)
        return NewPythonAwareParser(docClient), nil
    }

    // 类型断言
    docClient, ok := pythonClient.(*pyprovider.DocumentClient)
    if !ok {
        return nil, errors.New("invalid python client type")
    }

    return NewPythonAwareParser(docClient), nil
}