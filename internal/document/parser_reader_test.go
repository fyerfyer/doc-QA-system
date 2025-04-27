package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParserReaderImplementations(t *testing.T) {
	// 测试纯文本解析器
	t.Run("PlainText", func(t *testing.T) {
		content := "Hello, this is plain text."
		reader := strings.NewReader(content)

		parser := NewPlainTextParser()
		result, err := parser.ParseReader(reader, "test.txt")

		assert.NoError(t, err)
		assert.Equal(t, content, result)
	})

	// 测试Markdown解析器
	t.Run("Markdown", func(t *testing.T) {
		content := "# Heading\n\nThis is **markdown** text."
		reader := strings.NewReader(content)

		parser := NewMarkdownParser()
		result, err := parser.ParseReader(reader, "test.md")

		assert.NoError(t, err)
		assert.Contains(t, result, "Heading")
		assert.Contains(t, result, "markdown")
	})

	// PDF解析器需要真实的PDF文件，测试更复杂，这里略过
}

func TestPlainTextParserReader(t *testing.T) {
	parser := NewPlainTextParser()
	testContent := "This is test content.\nSecond line."
	reader := bytes.NewReader([]byte(testContent))

	result, err := parser.ParseReader(reader, "test.txt")
	assert.NoError(t, err)
	assert.Equal(t, testContent, result)
}

func TestMarkdownParserReader(t *testing.T) {
	parser := NewMarkdownParser()
	mdContent := "# Title\n\nThis is **bold** text."
	reader := bytes.NewReader([]byte(mdContent))

	result, err := parser.ParseReader(reader, "test.md")
	assert.NoError(t, err)
	assert.Contains(t, result, "Title")
	assert.Contains(t, result, "bold")
}
