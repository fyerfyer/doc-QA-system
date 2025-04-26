package document

import (
	"fmt"
	"strings"
	"unicode"
)

// SplitType 文本分段的类型
type SplitType string

const (
	// ByParagraph 按段落分割
	ByParagraph SplitType = "paragraph"
	// BySentence 按句子分割
	BySentence SplitType = "sentence"
	// ByLength 按字符长度分割
	ByLength SplitType = "length"
)

// SplitterConfig 分段器配置
type SplitterConfig struct {
	SplitType    SplitType // 分割类型
	ChunkSize    int       // 分块大小（按字符数）
	ChunkOverlap int       // 分块重叠大小（字符数）
	MaxChunks    int       // 最大分块数量（0表示不限制）
}

// DefaultSplitterConfig 返回默认分段器配置
func DefaultSplitterConfig() SplitterConfig {
	return SplitterConfig{
		SplitType:    ByParagraph,
		ChunkSize:    1000,
		ChunkOverlap: 200,
		MaxChunks:    0,
	}
}

// TextSplitter 实现文本分段器接口
type TextSplitter struct {
	config SplitterConfig
}

// NewTextSplitter 创建新的文本分段器
func NewTextSplitter(config SplitterConfig) *TextSplitter {
	return &TextSplitter{
		config: config,
	}
}

// Split 将文本分割成内容段落
func (s *TextSplitter) Split(text string) ([]Content, error) {
	if text == "" {
		return []Content{}, nil
	}

	var chunks []string

	switch s.config.SplitType {
	case ByParagraph:
		chunks = s.splitByParagraph(text)
		// 对于段落模式，跳过小块合并
		chunks = s.handleLargeChunks(chunks)
	case BySentence:
		chunks = s.splitBySentence(text)
		chunks = s.mergeSmallChunks(chunks)
		chunks = s.handleLargeChunks(chunks)
	case ByLength:
		chunks = s.splitByLength(text)
		chunks = s.mergeSmallChunks(chunks)
		chunks = s.handleLargeChunks(chunks)
	default:
		return nil, fmt.Errorf("unknown split type: %s", s.config.SplitType)
	}

	// 应用最大分块数量限制
	if s.config.MaxChunks > 0 && len(chunks) > s.config.MaxChunks {
		chunks = chunks[:s.config.MaxChunks]
	}

	// 构造Content对象
	var contents []Content
	for i, chunk := range chunks {
		contents = append(contents, Content{
			Text:  strings.TrimSpace(chunk),
			Index: i,
		})
	}

	return contents, nil
}

// splitByParagraph 按段落分割文本
func (s *TextSplitter) splitByParagraph(text string) []string {
	// 规范化段落分隔符
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// 按空行分段
	paragraphs := strings.Split(text, "\n\n")

	// 过滤空段落
	var result []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}

// splitBySentence 按句子分割文本
func (s *TextSplitter) splitBySentence(text string) []string {
	// 简单的句子分隔符
	sentenceDelimiters := []rune{'.', '!', '?', '；', '。', '！', '？'}

	var sentences []string
	var currentSentence strings.Builder

	for _, char := range text {
		currentSentence.WriteRune(char)

		// 检查是否是句子结束
		isSentenceEnd := false
		for _, delimiter := range sentenceDelimiters {
			if char == delimiter {
				isSentenceEnd = true
				break
			}
		}

		if isSentenceEnd {
			// 添加到句子列表
			sentence := strings.TrimSpace(currentSentence.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			currentSentence.Reset()
		}
	}

	// 处理最后一个可能不以分隔符结束的句子
	lastSentence := strings.TrimSpace(currentSentence.String())
	if lastSentence != "" {
		sentences = append(sentences, lastSentence)
	}

	return sentences
}

// splitByLength 按固定长度分割文本
func (s *TextSplitter) splitByLength(text string) []string {
	var chunks []string

	for i := 0; i < len(text); i += s.config.ChunkSize - s.config.ChunkOverlap {
		end := i + s.config.ChunkSize
		if end > len(text) {
			end = len(text)
		}

		// 寻找单词或自然边界结束
		if end < len(text) {
			// 尝试在空格处断开，避免单词被截断
			for end > i && end < len(text) && !unicode.IsSpace(rune(text[end])) {
				end--
			}

			// 如果找不到合适的空格，就在原位置截断
			if end <= i {
				end = i + s.config.ChunkSize
				if end > len(text) {
					end = len(text)
				}
			}
		}

		chunk := text[i:end]
		chunks = append(chunks, strings.TrimSpace(chunk))

		// 如果已经到达文本末尾，跳出循环
		if end == len(text) {
			break
		}
	}

	return chunks
}

// mergeSmallChunks 合并过小的段落
func (s *TextSplitter) mergeSmallChunks(chunks []string) []string {
	if len(chunks) <= 1 {
		return chunks
	}

	var result []string
	var currentChunk strings.Builder

	for _, chunk := range chunks {
		// 如果当前块加上新块不超过ChunkSize，则合并
		if currentChunk.Len()+len(chunk) <= s.config.ChunkSize {
			if currentChunk.Len() > 0 {
				currentChunk.WriteString(" ")
			}
			currentChunk.WriteString(chunk)
		} else {
			// 否则，保存当前块并开始新块
			if currentChunk.Len() > 0 {
				result = append(result, currentChunk.String())
			}
			currentChunk.Reset()
			currentChunk.WriteString(chunk)
		}
	}

	// 添加最后一个块
	if currentChunk.Len() > 0 {
		result = append(result, currentChunk.String())
	}

	return result
}

// handleLargeChunks 处理过长的段落
func (s *TextSplitter) handleLargeChunks(chunks []string) []string {
	var result []string

	for _, chunk := range chunks {
		// 如果段落长度超过最大值，按长度再分割
		if len(chunk) > s.config.ChunkSize {
			subChunks := s.splitByLength(chunk)
			result = append(result, subChunks...)
		} else {
			result = append(result, chunk)
		}
	}

	return result
}
