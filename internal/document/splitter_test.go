package document

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSplitByParagraph 测试按段落分割功能
func TestSplitByParagraph(t *testing.T) {
	config := DefaultSplitterConfig()
	splitter := NewTextSplitter(config)

	t.Run("basic paragraph splitting", func(t *testing.T) {
		text := "这是一个测试文档内容。\n\n这是第二段落。\n\n这是第三段落。"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(segments), "应该分割成3个段落")

		t.Logf("段落数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d: '%s'", i, seg.Text)
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

		t.Logf("单换行段落数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d: '%s'", i, seg.Text)
		}

		// 单换行通常会被合并为一个段落，除非有其他段落标记
		assert.LessOrEqual(t, len(segments), 3, "单换行应该被视为段落的一部分")
	})

	t.Run("markdown headers", func(t *testing.T) {
		text := "# 标题1\n\n这是第一部分内容。\n\n## 标题2\n\n这是第二部分内容。"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("带Markdown标题的段落数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d: '%s'", i, seg.Text)
		}

		// Markdown标题应该被识别为段落分隔符
		assert.GreaterOrEqual(t, len(segments), 3, "Markdown标题应该被分割为单独的段落")
	})
}

// TestSplitBySentence 测试按句子分割功能
func TestSplitBySentence(t *testing.T) {
	// 创建按句子分割的配置
	config := DefaultSplitterConfig()
	config.SplitType = BySentence
	splitter := NewTextSplitter(config)

	t.Run("basic sentence splitting", func(t *testing.T) {
		text := "这是第一个句子。这是第二个句子！这是第三个问题？"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("句子数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("句子 %d: '%s'", i, seg.Text)
		}

		// 验证句子分割
		assert.GreaterOrEqual(t, len(segments), 3, "应该至少分割成3个句子")
		// 验证第一个句子内容
		if len(segments) >= 1 {
			assert.Contains(t, segments[0].Text, "第一个句子")
		}
	})

	t.Run("chinese sentence splitting", func(t *testing.T) {
		text := "中文句子分割测试。这包含了中文的句号。还有问号？以及感叹号！"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("中文句子数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("句子 %d: '%s'", i, seg.Text)
		}

		// 验证中文句子分割
		assert.GreaterOrEqual(t, len(segments), 3, "应该至少分割成3个句子")
	})

	t.Run("mixed language sentences", func(t *testing.T) {
		text := "This is English. 这是中文。And this is mixed.混合语言句子测试！"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)

		t.Logf("混合语言句子数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("句子 %d: '%s'", i, seg.Text)
		}

		// 验证混合语言句子分割
		assert.GreaterOrEqual(t, len(segments), 3, "应该正确分割中英文混合的句子")
	})
}

// TestSplitByLength 测试按长度分割功能
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

		t.Logf("按长度分割后的段落数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d (长度=%d): '%s'...", i, len(seg.Text), seg.Text[:min(20, len(seg.Text))])
		}

		// 验证每个段落长度不超过ChunkSize
		for _, seg := range segments {
			assert.LessOrEqual(t, len(seg.Text), config.ChunkSize, "每个分段不应超过ChunkSize")
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

		t.Logf("带重叠的分段数量: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d: '%s'", i, seg.Text)
		}

		// 应该有两个段落，第二个段落开始应该包含第一个段落的结尾部分
		if len(segments) >= 2 {
			lastCharsOfFirst := segments[0].Text[len(segments[0].Text)-config.ChunkOverlap:]
			firstCharsOfSecond := segments[1].Text[:config.ChunkOverlap]
			assert.Equal(t, lastCharsOfFirst, firstCharsOfSecond, "段落之间应有指定的重叠")
		}
	})
}

// TestHandleLargeChunks 测试处理过长段落的功能
func TestHandleLargeChunks(t *testing.T) {
	config := DefaultSplitterConfig()
	config.ChunkSize = 50
	config.ChunkOverlap = 10
	splitter := NewTextSplitter(config)

	// 创建一个非常长的段落
	longParagraph := strings.Repeat("这是一个非常长的段落，需要被分割成更小的块。", 10)
	segments, err := splitter.Split(longParagraph)
	assert.NoError(t, err)

	t.Logf("长段落分割后的块数: %d", len(segments))
	for i, seg := range segments {
		t.Logf("块 %d (长度=%d): '%s'...", i, len(seg.Text), seg.Text[:min(20, len(seg.Text))])
	}

	// 验证长段落已经被分割成多个较小的块
	assert.Greater(t, len(segments), 1, "长段落应被分割成多个块")
	for _, seg := range segments {
		assert.LessOrEqual(t, len(seg.Text), config.ChunkSize+10, "每个块不应大幅超过ChunkSize")
	}
}

// TestCustomSplitterConfig 测试自定义分段器配置
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

		t.Logf("自定义块大小分割后段落数: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d (长度=%d): '%s'", i, len(seg.Text), seg.Text)
		}

		// 验证段落被适当分割
		assert.Greater(t, len(segments), 1, "应该被分割成多个块")
		for _, seg := range segments {
			assert.LessOrEqual(t, len(seg.Text), config.ChunkSize+5, "每个块不应大幅超过ChunkSize")
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

		t.Logf("应用最大块数限制后段落数: %d", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d: '%s'", i, seg.Text)
		}

		// 验证块数不超过MaxChunks
		assert.LessOrEqual(t, len(segments), config.MaxChunks, "段落数不应超过MaxChunks")
	})
}

// TestPreprocessText 测试文本预处理功能
func TestPreprocessText(t *testing.T) {
	splitter := NewTextSplitter(DefaultSplitterConfig())

	t.Run("normalize line endings", func(t *testing.T) {
		// 混合使用不同的换行符
		text := "行1\r\n行2\r行3\n行4"
		processed := splitter.preprocessText(text)

		// 应该将所有换行符统一为\n
		expected := "行1\n行2\n行3\n行4"
		assert.Equal(t, expected, processed, "应该将所有换行符规范化为\\n")
	})

	t.Run("remove excessive newlines", func(t *testing.T) {
		// 包含过多连续换行符的文本
		text := "段落1\n\n\n\n段落2\n\n\n段落3"
		processed := splitter.preprocessText(text)

		// 应该将连续的换行符减少到最多两个
		expected := "段落1\n\n段落2\n\n段落3"
		assert.Equal(t, expected, processed, "应该移除过多的连续换行符")
	})

	t.Run("trim whitespace", func(t *testing.T) {
		// 前后有空白的文本
		text := "  \t\n  测试文本\t \n  "
		processed := splitter.preprocessText(text)

		// 应该移除首尾的空白
		expected := "测试文本"
		assert.Equal(t, expected, processed, "应该移除首尾的空白")
	})
}

// TestEmptyInput 测试空输入的处理
func TestEmptyInput(t *testing.T) {
	splitter := NewTextSplitter(DefaultSplitterConfig())

	segments, err := splitter.Split("")
	assert.NoError(t, err)
	assert.Empty(t, segments, "空输入应返回空段落列表")

	segments, err = splitter.Split("   \n\t   ")
	assert.NoError(t, err)
	assert.Empty(t, segments, "只包含空白的输入应返回空段落列表")
}

// TestChineneTextSplitting 特别测试中文文本分割
func TestChineseTextSplitting(t *testing.T) {
	config := DefaultSplitterConfig()
	config.SplitType = BySentence
	splitter := NewTextSplitter(config)

	chineseText := "这是一段中文文本。它包含了几个句子。句子之间用中文句号分隔。" +
		"还可以使用问号吗？当然可以！中文感叹号也是支持的。"

	segments, err := splitter.Split(chineseText)
	require.NoError(t, err)

	t.Logf("中文句子分割结果，共 %d 个句子:", len(segments))
	for i, seg := range segments {
		t.Logf("句子 %d: '%s'", i, seg.Text)
	}

	// 应该分割成5个句子
	assert.GreaterOrEqual(t, len(segments), 4, "中文文本应该被正确分割成多个句子")

	// 验证第一个句子
	if len(segments) > 0 {
		assert.Contains(t, segments[0].Text, "这是一段中文文本")
	}
}

// TestEdgeCases 测试边缘情况
func TestEdgeCases(t *testing.T) {
	splitter := NewTextSplitter(DefaultSplitterConfig())

	t.Run("single character", func(t *testing.T) {
		segments, err := splitter.Split("A")
		assert.NoError(t, err)
		assert.Len(t, segments, 1, "单个字符应作为一个段落")
	})

	t.Run("only punctuation", func(t *testing.T) {
		segments, err := splitter.Split(".,!?;:")
		assert.NoError(t, err)
		assert.Len(t, segments, 1, "只包含标点符号的文本应作为一个段落")
	})

	t.Run("complex formatting", func(t *testing.T) {
		// 复杂格式的文本，包含列表项、引号等
		text := "# 标题\n\n* 项目1\n* 项目2\n\n> 引用文本\n\n```\n代码块\n```"
		segments, err := splitter.Split(text)
		assert.NoError(t, err)
		assert.NotEmpty(t, segments, "应能处理复杂格式的文本")

		t.Logf("复杂格式文本分割结果，共 %d 个段落:", len(segments))
		for i, seg := range segments {
			t.Logf("段落 %d: '%s'", i, seg.Text)
		}
	})
}

// min 返回两个整数中较小的一个
// Go 1.21之前没有内置的min函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
