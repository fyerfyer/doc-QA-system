package embedding

import (
	"context"
	"fmt"
	"sync"

	"github.com/gammazero/workerpool"
)

// DefaultBatchProcessor 默认批处理器
// 用于将大量文本分批处理以提高效率
type DefaultBatchProcessor struct {
	client     Client // 嵌入客户端
	batchSize  int    // 每批处理的文本数量
	maxWorkers int    // 最大并行工作线程数
	skipEmpty  bool   // 是否跳过空文本
}

// NewBatchProcessor 创建新的批处理器
func NewBatchProcessor(client Client, batchSize int, maxWorkers int) *DefaultBatchProcessor {
	if batchSize <= 0 {
		batchSize = 16 // 默认批量大小
	}

	if maxWorkers <= 0 {
		maxWorkers = 4 // 默认工作线程数
	}

	return &DefaultBatchProcessor{
		client:     client,
		batchSize:  batchSize,
		maxWorkers: maxWorkers,
		skipEmpty:  true,
	}
}

// Process 处理一批文本，将它们分成多个小批次并行处理
func (p *DefaultBatchProcessor) Process(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 过滤空文本（如果需要）
	var filteredTexts []string
	var emptyIndices []int // 记录空文本的位置

	if p.skipEmpty {
		filteredTexts = make([]string, 0, len(texts))
		for i, text := range texts {
			if text != "" {
				filteredTexts = append(filteredTexts, text)
			} else {
				emptyIndices = append(emptyIndices, i)
			}
		}
	} else {
		filteredTexts = texts
	}

	// 如果全是空文本
	if len(filteredTexts) == 0 {
		emptyResults := make([][]float32, len(texts))
		return emptyResults, nil
	}

	// 分割成批次
	batches := splitIntoBatches(filteredTexts, p.batchSize)

	// 创建工作池和结果收集器
	wp := workerpool.New(p.maxWorkers)
	resultsMu := sync.Mutex{}
	batchResults := make([]batchResult, len(batches))
	var processingErr error
	var errOnce sync.Once

	// 将任务提交到工作池
	for i, batch := range batches {
		i, batch := i, batch // 捕获循环变量
		wp.Submit(func() {
			// 检查上下文是否已取消
			select {
			case <-ctx.Done():
				errOnce.Do(func() {
					processingErr = ctx.Err()
				})
				return
			default:
				// 继续处理
			}

			// 调用嵌入客户端
			vectors, err := p.client.EmbedBatch(ctx, batch)

			resultsMu.Lock()
			defer resultsMu.Unlock()

			if err != nil {
				errOnce.Do(func() {
					processingErr = fmt.Errorf("batch %d processing error: %v", i, err)
				})
				return
			}

			batchResults[i] = batchResult{
				batchIndex: i,
				vectors:    vectors,
				err:        nil,
			}
		})
	}

	// 等待所有任务完成
	wp.StopWait()

	// 检查是否有错误发生
	if processingErr != nil {
		return nil, processingErr
	}

	// 合并所有批次结果为一个列表
	var allVectors [][]float32
	for _, br := range batchResults {
		allVectors = append(allVectors, br.vectors...)
	}

	// 如果有空文本，需要将结果重新插回对应位置
	if len(emptyIndices) > 0 {
		finalResults := make([][]float32, len(texts))
		vectorIndex := 0

		for i := 0; i < len(texts); i++ {
			if containsInt(emptyIndices, i) {
				finalResults[i] = nil // 对于空文本返回nil
			} else {
				if vectorIndex < len(allVectors) {
					finalResults[i] = allVectors[vectorIndex]
					vectorIndex++
				}
			}
		}

		return finalResults, nil
	}

	return allVectors, nil
}

// batchResult 表示一个批处理的结果
type batchResult struct {
	batchIndex int
	vectors    [][]float32
	err        error
}

// splitIntoBatches 将文本列表分割成多个批次
func splitIntoBatches(texts []string, batchSize int) [][]string {
	if batchSize <= 0 {
		batchSize = 1
	}

	batches := make([][]string, 0, (len(texts)+batchSize-1)/batchSize)

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batches = append(batches, texts[i:end])
	}

	return batches
}

// containsInt 检查整数切片中是否包含特定值
func containsInt(slice []int, val int) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
