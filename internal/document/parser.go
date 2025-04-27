package document

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
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

// ContentType 表示文档的内容类型
type ContentType string

const (
	// PDF 文档类型
	PDF ContentType = "pdf"
	// Markdown 文档类型
	Markdown ContentType = "markdown"
	// PlainText 纯文本类型
	PlainText ContentType = "plaintext"
	// Unknown 未知类型
	Unknown ContentType = "unknown"
)

// ParserFactory 解析器工厂函数，根据文件类型创建对应的解析器
func ParserFactory(filePath string) (Parser, error) {
	contentType := detectContentType(filePath)

	switch contentType {
	case PDF:
		return NewPDFParser(), nil
	case Markdown:
		return NewMarkdownParser(), nil
	case PlainText:
		return NewPlainTextParser(), nil
	default:
		return nil, errors.New("unsupported document type")
	}
}

// detectContentType 根据文件扩展名检测内容类型
func detectContentType(filePath string) ContentType {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".pdf":
		return PDF
	case ".md", ".markdown":
		return Markdown
	case ".txt":
		return PlainText
	default:
		return Unknown
	}
}

// Document 解析后的文档结构
type Document struct {
	Content string            // 文档文本内容
	Title   string            // 文档标题（可选）
	Source  string            // 源文件信息
	Meta    map[string]string // 元数据（可选，例如作者、日期等）
}

// Content 表示文档的内容段落
type Content struct {
	Text  string // 段落文本内容
	Index int    // 段落索引
}

// Splitter 文本分段器接口
// 负责将长文本分割成适合向量化的小段
type Splitter interface {
	// Split 将文本分割成段落
	Split(text string) ([]Content, error)
}
