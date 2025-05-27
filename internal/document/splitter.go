package document

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