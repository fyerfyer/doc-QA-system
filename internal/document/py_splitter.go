package document

import (
	"context"
	"errors"
	"fmt"

	"github.com/fyerfyer/doc-QA-system/internal/pyprovider"
)

// PythonSplitter 使用Python服务的文本分块器
type PythonSplitter struct {
	client     *pyprovider.DocumentClient // Python文档客户端
	chunkSize  int                        // 块大小
	overlap    int                        // 重叠大小
	splitType  string                     // 分割类型
	documentID string                     // 文档ID,可选
}

// SplitConfig 分块器配置
type SplitConfig struct {
	ChunkSize  int    // 块大小
	Overlap    int    // 重叠大小
	SplitType  string // 分割类型
	DocumentID string // 文档ID,可选
}

// DefaultSplitterConfig 返回默认的分块器配置
func DefaultSplitterConfig() SplitConfig {
	return SplitConfig{
		ChunkSize:  1000,
		Overlap:    200,
		SplitType:  "sentence",
		DocumentID: "",
	}
}

// NewTextSplitter 创建文本分块器
func NewTextSplitter(config SplitConfig) (Splitter, error) {
	// 这里创建一个默认客户端
	httpClient, err := pyprovider.NewClient(pyprovider.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create python client: %w", err)
	}

	docClient := pyprovider.NewDocumentClient(httpClient)
	return NewPythonSplitter(docClient, config), nil
}

// NewPythonSplitter 创建Python分块器
func NewPythonSplitter(client *pyprovider.DocumentClient, config SplitConfig) Splitter {
	return &PythonSplitter{
		client:     client,
		chunkSize:  config.ChunkSize,
		overlap:    config.Overlap,
		splitType:  config.SplitType,
		documentID: config.DocumentID,
	}
}

// Split 将文本分割成段落
func (s *PythonSplitter) Split(text string) ([]Content, error) {
	if s.client == nil {
		return nil, errors.New("python client uninitialized")
	}

	// 生成临时文档ID（如果未提供）
	documentID := s.documentID
	if documentID == "" {
		documentID = fmt.Sprintf("temp_%d", len(text)%10000)
	}

	// 创建分块选项
	options := &pyprovider.SplitOptions{
		ChunkSize:    s.chunkSize,
		ChunkOverlap: s.overlap,
		SplitType:    s.splitType,
		StoreResult:  true,
	}

	// 调用Python服务分块文本
	ctx := context.Background()
	pyContents, _, err := s.client.SplitText(ctx, text, documentID, options)
	if err != nil {
		// TODO: 实现回退机制，在Python分块失败时尝试本地分块
		return nil, fmt.Errorf("failed to split document by python: %w", err)
	}

	// 将Python的Content结构转换为本地的Content结构
	contents := make([]Content, len(pyContents))
	for i, pyContent := range pyContents {
		contents[i] = Content{
			Text:  pyContent.Text,
			Index: pyContent.Index,
		}
	}

	return contents, nil
}

// GetChunkSize 返回块大小
func (s *PythonSplitter) GetChunkSize() int {
	return s.chunkSize
}

// GetOverlap 返回重叠大小
func (s *PythonSplitter) GetOverlap() int {
	return s.overlap
}

// GetSplitType 返回分割类型
func (s *PythonSplitter) GetSplitType() string {
	return s.splitType
}
