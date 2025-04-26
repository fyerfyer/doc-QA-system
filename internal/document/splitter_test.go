package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitByParagraph(t *testing.T) {
	config := DefaultSplitterConfig()
	splitter := NewTextSplitter(config)

	text1 := "这是一个测试文档内容。\n\n这是第二段落。\n\n这是第三段落。"
	segments1, err := splitter.Split(text1)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(segments1), "Should split into 3 segments with double newlines")

	t.Logf("Double newline segments count: %d", len(segments1))
	for i, seg := range segments1 {
		t.Logf("Segment %d: '%s'", i, seg.Text)
	}

	text2 := "这是一个测试文档内容。\n这是第二段落。\n这是第三段落。"
	segments2, err := splitter.Split(text2)
	assert.NoError(t, err)

	t.Logf("Single newline segments count: %d", len(segments2))
	for i, seg := range segments2 {
		t.Logf("Segment %d: '%s'", i, seg.Text)
	}
}
