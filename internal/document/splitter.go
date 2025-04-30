package document

import (
	"fmt"
	"regexp"
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
	// BySemantic 按语义分割（需要更复杂的实现，后期添加）
	BySemantic SplitType = "semantic"
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

	// 预处理文本：规范化换行符、移除多余空白等
	text = s.preprocessText(text)

	var chunks []string

	switch s.config.SplitType {
	case ByParagraph:
		chunks = s.splitByParagraph(text)
		// 对段落进行进一步处理，确保不会过长
		chunks = s.handleLargeChunks(chunks)
	case BySentence:
		chunks = s.splitBySentence(text)
		chunks = s.mergeSmallChunks(chunks)
		chunks = s.handleLargeChunks(chunks)
	case ByLength:
		chunks = s.splitByLength(text)
	default:
		return nil, fmt.Errorf("unknown split type: %s", s.config.SplitType)
	}

	// 过滤空段落和进行最终清理
	chunks = s.filterAndCleanChunks(chunks)

	// 应用最大分块数量限制
	if s.config.MaxChunks > 0 && len(chunks) > s.config.MaxChunks {
		chunks = chunks[:s.config.MaxChunks]
	}

	// 构造Content对象
	var contents []Content
	for i, chunk := range chunks {
		contents = append(contents, Content{
			Text:  chunk,
			Index: i,
		})
	}

	return contents, nil
}

// preprocessText 预处理文本，规范化格式
func (s *TextSplitter) preprocessText(text string) string {
	// 统一换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// 移除连续的空白行，最多保留两个换行符
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}

// splitByParagraph 按段落分割文本
func (s *TextSplitter) splitByParagraph(text string) []string {
	// 使用正则表达式匹配段落分隔
	// 更智能地识别段落边界：空行、markdown标题格式等
	// 中英文混合文档的段落处理
	paragraphSplitter := regexp.MustCompile(`(?m)^\s*$|^#{1,6}\s|^\*\s`)
	parts := paragraphSplitter.Split(text, -1)

	var paragraphs []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			paragraphs = append(paragraphs, part)
		}
	}

	// 如果段落识别结果不理想（太少或没有），回退到简单的换行符分割
	if len(paragraphs) <= 1 {
		paragraphs = []string{}
		simpleParas := strings.Split(text, "\n\n")
		for _, para := range simpleParas {
			para = strings.TrimSpace(para)
			if para != "" {
				paragraphs = append(paragraphs, para)
			}
		}

		// 如果还是没有段落，则按单行拆分
		if len(paragraphs) <= 1 && len(text) > s.config.ChunkSize {
			simpleParas = strings.Split(text, "\n")
			for _, para := range simpleParas {
				para = strings.TrimSpace(para)
				if para != "" {
					paragraphs = append(paragraphs, para)
				}
			}
		}
	}

	return paragraphs
}

// splitBySentence 按句子分割文本
func (s *TextSplitter) splitBySentence(text string) []string {
	// 改进句子分隔符识别，增强对中文文本的支持
	// 支持更多标点符号：英文句号、问号、感叹号，以及中文句号、问号、感叹号等
	sentenceDelimiters := []string{".", "!", "?", "。", "！", "？", "；", ";"}

	var sentences []string
	var currentSentence strings.Builder
	var inQuote bool // 跟踪是否在引号内部

	// 一次扫描文本，构建句子
	for _, char := range text {
		currentSentence.WriteRune(char)

		// 判断引号状态
		if char == '"' || char == '"' || char == '"' {
			inQuote = !inQuote
		}

		// 检查是否是句子结束（不在引号内部时）
		if !inQuote {
			isSentenceEnd := false
			charStr := string(char)
			for _, delimiter := range sentenceDelimiters {
				if charStr == delimiter {
					isSentenceEnd = true
					break
				}
			}

			// 如果是句子结束，且下一个字符是空格或换行等
			if isSentenceEnd {
				sentence := strings.TrimSpace(currentSentence.String())
				if sentence != "" {
					sentences = append(sentences, sentence)
					currentSentence.Reset()
				}
			}
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

	// 对于按字符分割，需要考虑不切断单词和词组
	for i := 0; i < len(text); i += s.config.ChunkSize - s.config.ChunkOverlap {
		end := i + s.config.ChunkSize
		if end > len(text) {
			end = len(text)
		}

		// 寻找单词边界或句子边界作为截断点
		if end < len(text) {
			// 优先寻找句子结束的位置（句号、问号、感叹号等）
			sentenceEndPos := s.findSentenceEnd(text, i, end)
			if sentenceEndPos > i {
				end = sentenceEndPos
			} else {
				// 其次寻找段落边界（换行符）
				paraEndPos := strings.LastIndex(text[i:end], "\n")
				if paraEndPos > 0 {
					end = i + paraEndPos + 1
				} else {
					// 最后尝试在单词边界截断
					wordEndPos := s.findWordBoundary(text, i, end)
					if wordEndPos > i {
						end = wordEndPos
					}
				}
			}
		}

		chunk := text[i:end]
		chunks = append(chunks, strings.TrimSpace(chunk))

		if end == len(text) {
			break
		}
	}

	return chunks
}

// findSentenceEnd 在指定范围内查找句子结束的位置
func (s *TextSplitter) findSentenceEnd(text string, start, end int) int {
	sentenceEnders := []string{".", "!", "?", "。", "！", "？"}
	for i := end - 1; i >= start; i-- {
		for _, ender := range sentenceEnders {
			if i+len(ender) <= len(text) && text[i:i+len(ender)] == ender {
				return i + len(ender)
			}
		}
	}
	return -1
}

// findWordBoundary 寻找适合的单词边界
func (s *TextSplitter) findWordBoundary(text string, start, end int) int {
	// 从末尾向前查找空格或标点
	for i := end - 1; i >= start+s.config.ChunkSize/2; i-- {
		r := rune(text[i])
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return i + 1
		}
	}

	// 找不到合适的边界，就使用原始截断点
	return end
}

// mergeSmallChunks 合并过小的段落
func (s *TextSplitter) mergeSmallChunks(chunks []string) []string {
	if len(chunks) <= 1 {
		return chunks
	}

	// 定义小块的阈值，如果段落小于这个值就考虑合并
	smallChunkThreshold := s.config.ChunkSize / 5

	var result []string
	var currentChunk strings.Builder
	currentSize := 0

	for _, chunk := range chunks {
		chunkSize := len(chunk)

		// 如果当前块加上新块不超过ChunkSize，则合并
		if currentSize+chunkSize <= s.config.ChunkSize {
			if currentChunk.Len() > 0 && !strings.HasSuffix(currentChunk.String(), "\n") {
				currentChunk.WriteString(" ")
			}
			currentChunk.WriteString(chunk)
			currentSize += chunkSize
		} else {
			// 否则，保存当前块并开始新块
			if currentChunk.Len() > 0 {
				result = append(result, currentChunk.String())
			}
			currentChunk.Reset()
			currentChunk.WriteString(chunk)
			currentSize = chunkSize
		}

		// 如果当前块已经接近目标大小，直接添加到结果
		if currentSize >= s.config.ChunkSize*4/5 && currentSize > smallChunkThreshold {
			result = append(result, currentChunk.String())
			currentChunk.Reset()
			currentSize = 0
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
		// 如果段落长度超过最大值，进行智能分割
		if len(chunk) > s.config.ChunkSize {
			// 根据段落内容选择合适的分割方式
			if strings.Count(chunk, "\n") > 3 {
				// 如果有多个换行符，尝试按换行符分割
				subChunks := strings.Split(chunk, "\n")
				subChunks = s.mergeSmallChunks(subChunks)
				result = append(result, subChunks...)
			} else if containsChineseSentences(chunk) {
				// 如果包含中文句子，尝试按中文句子分割
				subChunks := s.splitChineseSentences(chunk)
				subChunks = s.mergeSmallChunks(subChunks)
				result = append(result, subChunks...)
			} else {
				// 其他情况按长度分割
				subChunks := s.splitByLength(chunk)
				result = append(result, subChunks...)
			}
		} else {
			result = append(result, chunk)
		}
	}

	return result
}

// splitChineseSentences 专门处理中文句子的分割
func (s *TextSplitter) splitChineseSentences(text string) []string {
	// 中文句子分隔符
	separators := []rune{'。', '！', '？', '；', '.', '!', '?', ';'}

	var sentences []string
	var currentSentence strings.Builder

	for _, char := range text {
		currentSentence.WriteRune(char)

		// 检查是否是句子结束
		isSentenceEnd := false
		for _, sep := range separators {
			if char == sep {
				isSentenceEnd = true
				break
			}
		}

		if isSentenceEnd {
			sentences = append(sentences, strings.TrimSpace(currentSentence.String()))
			currentSentence.Reset()
		}
	}

	// 处理最后一个句子
	if currentSentence.Len() > 0 {
		sentences = append(sentences, strings.TrimSpace(currentSentence.String()))
	}

	return sentences
}

// filterAndCleanChunks 过滤空段落并进行最终清理
func (s *TextSplitter) filterAndCleanChunks(chunks []string) []string {
	var result []string

	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			result = append(result, chunk)
		}
	}

	return result
}

// containsChineseSentences 判断文本是否包含中文句子
func containsChineseSentences(text string) bool {
	// 检查是否包含中文字符
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
