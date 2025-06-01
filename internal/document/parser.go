package document

import (
    "context"
    "fmt"
    "io"

    "github.com/fyerfyer/doc-QA-system/internal/pyprovider"
)

// Parser 文档解析器接口
// 负责将不同格式的文档解析为纯文本
type Parser interface {
    // Parse 解析文档，返回文本内容
    Parse(filePath string) (string, error)

    // ParseReader 从Reader解析文档，返回文本内容
    // filename用于确定文档类型
    ParseReader(r io.Reader, filename string) (string, error)
}

// PythonAwareParser 支持Python API调用的增强解析器接口
// 继承基本Parser接口，增加上下文相关方法
type PythonAwareParser interface {
    Parser

    // ParseWithContext 使用上下文解析文档
    ParseWithContext(ctx context.Context, filePath string) (string, error)
    
    // ParseReaderWithContext 使用上下文从Reader解析文档
    ParseReaderWithContext(ctx context.Context, r io.Reader, filename string) (string, error)
}

// ParserFactory 根据文件类型创建对应的解析器
// 现在所有文件解析都委托给Python服务
func ParserFactory(filePath string) (Parser, error) {
    // 检查文件路径
    if filePath == "" {
        return nil, fmt.Errorf("invalid file path")
    }
    
    // 创建默认的Python客户端
    config := pyprovider.DefaultConfig()
    httpClient, err := pyprovider.NewClient(config)
    if err != nil {
        return nil, fmt.Errorf("failed to create python client: %w", err)
    }
    
    // 创建文档客户端
    docClient := pyprovider.NewDocumentClient(httpClient)
    
    // 创建并返回Python解析器
    return NewPythonParser(docClient), nil
}