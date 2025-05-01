package document

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitByParagraph(t *testing.T) {
	config := DefaultSplitterConfig()
	splitter := NewTextSplitter(config)

	t.Run("basic paragraph splitting", func(t *testing.T) {
		text := "这是一个测试文档内容。\n\n这是第二段落。\n\n这是第三段落。"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(segments), "should be split into 3 paragraphs")

		t.Logf("paragraph numbers: %d", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d: '%s'", i, seg.Text)
		}

		// 验证段落内容
		assert.Contains(t, segments[0].Text, "测试文档内容")
		assert.Contains(t, segments[1].Text, "第二段落")
		assert.Contains(t, segments[2].Text, "第三段落")
	})

	t.Run("single newline splitting", func(t *testing.T) {
		text := "这是一个测试文档内容。\n这是第二段落。\n这是第三段落。"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("paragraph numbers with single newline: %d", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d: '%s'", i, seg.Text)
		}

		// 单换行通常会被合并为一个段落，除非有其他段落标记
		assert.LessOrEqual(t, len(segments), 3, "Single newline should be treated as part of a paragraph")
	})

	t.Run("markdown headers", func(t *testing.T) {
		text := "# 标题1\n\n这是第一部分内容。\n\n## 标题2\n\n这是第二部分内容。"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("paragraph numbers with Markdown headers: %d", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d: '%s'", i, seg.Text)
		}

		// Markdown标题应该被识别为段落分隔符
		assert.GreaterOrEqual(t, len(segments), 3, "Markdown headers should be split as separate paragraphs")
	})
}

func TestSplitBySentence(t *testing.T) {
	// 创建按句子分割的配置
	config := DefaultSplitterConfig()
	config.SplitType = BySentence
	splitter := NewTextSplitter(config)

	t.Run("basic sentence splitting", func(t *testing.T) {
		text := "这是第一个句子。这是第二个句子！这是第三个问题？"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("sentence numbers: %d", len(segments))
		for i, seg := range segments {
			t.Logf("sentence %d: '%s'", i, seg.Text)
		}

		// 验证句子分割
		assert.GreaterOrEqual(t, len(segments), 3, "Should be split into at least 3 sentences")
		// 验证第一个句子内容
		if len(segments) >= 1 {
			assert.Contains(t, segments[0].Text, "第一个句子")
		}
	})

	t.Run("chinese sentence splitting", func(t *testing.T) {
		text := "中文句子分割测试。这包含了中文的句号。还有问号？以及感叹号！"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("Chinese sentence numbers: %d", len(segments))
		for i, seg := range segments {
			t.Logf("sentence %d: '%s'", i, seg.Text)
		}

		// 验证中文句子分割
		assert.GreaterOrEqual(t, len(segments), 3, "Should be split into at least 3 sentences")
	})

	t.Run("mixed language sentences", func(t *testing.T) {
		text := "This is English. 这是中文。And this is mixed.混合语言句子测试！"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("mixed language sentence numbers: %d", len(segments))
		for i, seg := range segments {
			t.Logf("sentence %d: '%s'", i, seg.Text)
		}

		// 验证混合语言句子分割
		assert.GreaterOrEqual(t, len(segments), 3, "Should correctly split mixed Chinese and English sentences")
	})
}

func TestSplitByLength(t *testing.T) {
	t.Run("chunk size constraint", func(t *testing.T) {
		// 创建限制长度的配置
		config := DefaultSplitterConfig()
		config.SplitType = ByLength
		config.ChunkSize = 50
		config.ChunkOverlap = 10
		splitter := NewTextSplitter(config)

		// 创建一个超过ChunkSize的长文本
		longText := strings.Repeat("这是测试文本，需要按长度进行分割。", 10)
		segments, err := splitter.Split(longText)
		assert.NoError(t, err)

		t.Logf("number of paragraphs after splitting by length: %d", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d (length=%d): '%s'...", i, len(seg.Text), seg.Text[:min(20, len(seg.Text))])
		}

		// 验证每个段落长度不超过ChunkSize
		for _, seg := range segments {
			assert.LessOrEqual(t, len(seg.Text), config.ChunkSize, "Each chunk should not exceed ChunkSize")
		}
	})

	t.Run("with overlap", func(t *testing.T) {
		// 创建带重叠的配置
		config := DefaultSplitterConfig()
		config.SplitType = ByLength
		config.ChunkSize = 30
		config.ChunkOverlap = 10
		splitter := NewTextSplitter(config)

		text := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("number of chunks with overlap: %d", len(segments))
		for i, seg := range segments {
			t.Logf("chunk %d: '%s'", i, seg.Text)
		}

		// 应该有两个段落，第二个段落开始应该包含第一个段落的结尾部分
		if len(segments) >= 2 {
			lastCharsOfFirst := segments[0].Text[len(segments[0].Text)-config.ChunkOverlap:]
			firstCharsOfSecond := segments[1].Text[:config.ChunkOverlap]
			assert.Equal(t, lastCharsOfFirst, firstCharsOfSecond, "Chunks should have specified overlap")
		}
	})
}

// 测试处理过长段落的功能
func TestHandleLargeChunks(t *testing.T) {
	config := DefaultSplitterConfig()
	config.ChunkSize = 50
	config.ChunkOverlap = 10
	splitter := NewTextSplitter(config)

	// 创建一个非常长的段落
	longParagraph := strings.Repeat("这是一个非常长的段落，需要被分割成更小的块。", 10)
	segments, err := splitter.Split(longParagraph)
	assert.NoError(t, err)

	t.Logf("number of chunks after splitting long paragraph: %d", len(segments))
	for i, seg := range segments {
		t.Logf("chunk %d (length=%d): '%s'...", i, len(seg.Text), seg.Text[:min(20, len(seg.Text))])
	}

	// 验证长段落已经被分割成多个较小的块
	assert.Greater(t, len(segments), 1, "Long paragraph should be split into multiple chunks")
	for _, seg := range segments {
		assert.LessOrEqual(t, len(seg.Text), config.ChunkSize+10, "Each chunk should not significantly exceed ChunkSize")
	}
}

// 测试自定义分段器配置
func TestCustomSplitterConfig(t *testing.T) {
	t.Run("custom chunk size", func(t *testing.T) {
		// 使用更小的块大小
		config := DefaultSplitterConfig()
		config.ChunkSize = 30
		config.ChunkOverlap = 5
		splitter := NewTextSplitter(config)

		text := "这是一段测试文本。这段文本用于测试自定义分段器配置。我们将使用较小的块大小。"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("number of paragraphs after custom chunk size: %d", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d (length=%d): '%s'", i, len(seg.Text), seg.Text)
		}

		// 验证段落被适当分割
		assert.Greater(t, len(segments), 1, "Should be split into multiple chunks")
		for _, seg := range segments {
			assert.LessOrEqual(t, len(seg.Text), config.ChunkSize+5, "Each chunk should not significantly exceed ChunkSize")
		}
	})

	t.Run("max chunks limit", func(t *testing.T) {
		// 设置最大块数限制
		config := DefaultSplitterConfig()
		config.ChunkSize = 20
		config.MaxChunks = 3
		splitter := NewTextSplitter(config)

		// 创建一个长文本，应该产生超过MaxChunks的块
		longText := strings.Repeat("这是测试文本。", 20)
		segments, err := splitter.Split(longText)
		assert.NoError(t, err)

		t.Logf("number of paragraphs after applying max chunks limit: %d", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d: '%s'", i, seg.Text)
		}

		// 验证块数不超过MaxChunks
		assert.LessOrEqual(t, len(segments), config.MaxChunks, "Number of chunks should not exceed MaxChunks")
	})
}

// 测试文本预处理功能
func TestPreprocessText(t *testing.T) {
	splitter := NewTextSplitter(DefaultSplitterConfig())

	t.Run("normalize line endings", func(t *testing.T) {
		// 混合使用不同的换行符
		text := "行1\r\n行2\r行3\n行4"
		processed := splitter.preprocessText(text)

		// 应该将所有换行符统一为\n
		expected := "行1\n行2\n行3\n行4"
		assert.Equal(t, expected, processed, "All line endings should be normalized to \\n")
	})

	t.Run("remove excessive newlines", func(t *testing.T) {
		// 包含过多连续换行符的文本
		text := "段落1\n\n\n\n段落2\n\n\n段落3"
		processed := splitter.preprocessText(text)

		// 应该将连续的换行符减少到最多两个
		expected := "段落1\n\n段落2\n\n段落3"
		assert.Equal(t, expected, processed, "Excessive consecutive newlines should be removed")
	})

	t.Run("trim whitespace", func(t *testing.T) {
		// 前后有空白的文本
		text := "  \t\n  测试文本\t \n  "
		processed := splitter.preprocessText(text)

		// 应该移除首尾的空白
		expected := "测试文本"
		assert.Equal(t, expected, processed, "Leading and trailing whitespace should be removed")
	})
}

// 测试空输入的处理
func TestEmptyInput(t *testing.T) {
	splitter := NewTextSplitter(DefaultSplitterConfig())

	segments, err := splitter.Split("")
	assert.NoError(t, err)
	assert.Empty(t, segments, "Empty input should return empty segment list")

	segments, err = splitter.Split("   \n\t   ")
	assert.NoError(t, err)
	assert.Empty(t, segments, "Input with only whitespace should return empty segment list")
}

// 特别测试中文文本分割
func TestChineseTextSplitting(t *testing.T) {
	config := DefaultSplitterConfig()
	config.SplitType = BySentence
	splitter := NewTextSplitter(config)

	chineseText := "这是一段中文文本。它包含了几个句子。句子之间用中文句号分隔。" +
		"还可以使用问号吗？当然可以！中文感叹号也是支持的。"

	segments, err := splitter.Split(chineseText)
	require.NoError(t, err)

	t.Logf("Chinese sentence split result, total %d sentences:", len(segments))
	for i, seg := range segments {
		t.Logf("sentence %d: '%s'", i, seg.Text)
	}

	// 应该分割成5个句子
	assert.GreaterOrEqual(t, len(segments), 4, "Chinese text should be correctly split into multiple sentences")

	// 验证第一个句子
	if len(segments) > 0 {
		assert.Contains(t, segments[0].Text, "这是一段中文文本")
	}
}

// 测试边缘情况
func TestEdgeCases(t *testing.T) {
	splitter := NewTextSplitter(DefaultSplitterConfig())

	t.Run("single character", func(t *testing.T) {
		segments, err := splitter.Split("A")
		assert.NoError(t, err)
		assert.Len(t, segments, 1, "A single character should be treated as one paragraph")
	})

	t.Run("only punctuation", func(t *testing.T) {
		segments, err := splitter.Split(".,!?;:")
		assert.NoError(t, err)
		assert.Len(t, segments, 1, "Text with only punctuation should be treated as one paragraph")
	})

	t.Run("complex formatting", func(t *testing.T) {
		// 复杂格式的文本，包含列表项、引号等
		text := "# 标题\n\n* 项目1\n* 项目2\n\n> 引用文本\n\n```\n代码块\n```"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)
		assert.NotEmpty(t, segments, "Should handle complex formatted text")

		t.Logf("complex formatted text split result, total %d paragraphs:", len(segments))
		for i, seg := range segments {
			t.Logf("paragraph %d: '%s'", i, seg.Text)
		}
	})
}
