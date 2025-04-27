package document

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// MarkdownParser Markdown文档解析器
type MarkdownParser struct{}

// NewMarkdownParser 创建新的Markdown解析器
func NewMarkdownParser() Parser {
	return &MarkdownParser{}
}

// Parse 解析Markdown文件并提取文本内容
func (p *MarkdownParser) Parse(filePath string) (string, error) {
	// 读取文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open markdown file: %v", err)
	}
	defer file.Close()

	// 使用ParseReader实现
	return p.ParseReader(file, filePath)
}

// ParseReader 从Reader解析Markdown内容
func (p *MarkdownParser) ParseReader(r io.Reader, filename string) (string, error) {
	// 读取文件内容
	content, err := ioutil.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read markdown content: %v", err)
	}

	// 创建Markdown解析器
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	mdParser := parser.NewWithExtensions(extensions)

	// 解析Markdown内容
	doc := mdParser.Parse(content)

	// 创建HTML渲染器
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	renderer := html.NewRenderer(html.RendererOptions{Flags: htmlFlags})

	// 将Markdown转换为HTML
	htmlContent := markdown.Render(doc, renderer)

	// 从HTML中提取纯文本（简单处理，移除HTML标签）
	plainText := extractTextFromHTML(string(htmlContent))

	return plainText, nil
}

// extractTextFromHTML 从HTML中提取纯文本
// 注意：这是一个简化的实现，更复杂的情况可能需要使用HTML解析库
func extractTextFromHTML(html string) string {
	// 替换常见的HTML元素为空格或换行符
	replacements := []struct {
		Old string
		New string
	}{
		{"<br>", "\n"},
		{"<br/>", "\n"},
		{"<br />", "\n"},
		{"<p>", ""},
		{"</p>", "\n\n"},
		{"<li>", "- "},
		{"</li>", "\n"},
		{"<ul>", "\n"},
		{"</ul>", "\n"},
		{"<ol>", "\n"},
		{"</ol>", "\n"},
		{"<h1>", "\n\n"},
		{"</h1>", "\n\n"},
		{"<h2>", "\n\n"},
		{"</h2>", "\n\n"},
		{"<h3>", "\n\n"},
		{"</h3>", "\n\n"},
		{"<h4>", "\n\n"},
		{"</h4>", "\n\n"},
		{"<h5>", "\n\n"},
		{"</h5>", "\n\n"},
		{"<h6>", "\n\n"},
		{"</h6>", "\n\n"},
	}

	result := html
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.Old, r.New)
	}

	// 移除所有HTML标签
	for {
		start := strings.Index(result, "<")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		result = result[:start] + " " + result[start+end+1:]
	}

	// 规范化空白
	result = normalizeWhitespace(result)

	return result
}

// normalizeWhitespace 规范化文本中的空白符
func normalizeWhitespace(text string) string {
	// 替换连续的空白符为单个空格
	text = strings.Join(strings.Fields(text), " ")

	// 替换连续多个换行符为最多两个
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}
