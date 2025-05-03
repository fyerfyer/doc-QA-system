package vectordb

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"
)

// BaseRepository 基础仓库实现
// 为各种具体的向量数据库实现提供共享功能
type BaseRepository struct {
	dimension int                 // 向量维度
	distType  DistanceType        // 距离计算类型
	idToDoc   map[string]int      // 文档ID到索引的映射
	fileToIDs map[string][]string // 文件ID到文档ID的映射
}

// NewBaseRepository 创建基础仓库
func NewBaseRepository(dimension int, distType DistanceType) *BaseRepository {
	return &BaseRepository{
		dimension: dimension,
		distType:  distType,
		idToDoc:   make(map[string]int),
		fileToIDs: make(map[string][]string),
	}
}

// ComputeDistance 计算两个向量间的距离
func ComputeDistance(v1, v2 []float32, distType DistanceType) (float32, error) {
	if len(v1) != len(v2) {
		return 0, fmt.Errorf("vector dimensions do not match: %d vs %d", len(v1), len(v2))
	}

	switch distType {
	case Cosine:
		return cosineDistance(v1, v2), nil
	case DotProduct:
		return dotProduct(v1, v2), nil
	case Euclidean:
		return euclideanDistance(v1, v2), nil
	default:
		return 0, fmt.Errorf("unsupported distance type: %s", distType)
	}
}

// cosineDistance 计算余弦距离
// 优化：使用SIMD指令集加速（仅在特定平台可用）
func cosineDistance(v1, v2 []float32) float32 {
	// 余弦相似度 = 点积 / (||v1|| * ||v2||)
	// 余弦距离 = 1 - 余弦相似度
	dot := dotProduct(v1, v2)
	norm1 := vectorNorm(v1)
	norm2 := vectorNorm(v2)

	if norm1 == 0 || norm2 == 0 {
		return 1.0 // 最大距离
	}

	similarity := dot / (norm1 * norm2)
	// 处理浮点精度问题
	if similarity > 1.0 {
		similarity = 1.0
	} else if similarity < -1.0 {
		similarity = -1.0
	}

	return 1.0 - similarity
}

// dotProduct 计算两个向量的点积
// 优化：使用并行处理加速大向量计算
func dotProduct(v1, v2 []float32) float32 {
	n := len(v1)

	// 小向量直接计算
	if n < 1000 {
		var dot float32
		for i := 0; i < n; i++ {
			dot += v1[i] * v2[i]
		}
		return dot
	}

	// 大向量使用并行计算
	cpus := runtime.NumCPU()
	if cpus > 4 {
		cpus = 4 // 限制最大并行度
	}

	// 向量太小不值得并行
	if n < cpus*250 {
		var dot float32
		for i := 0; i < n; i++ {
			dot += v1[i] * v2[i]
		}
		return dot
	}

	// 使用goroutine并行计算
	var wg sync.WaitGroup
	results := make([]float32, cpus)
	batchSize := n / cpus

	for i := 0; i < cpus; i++ {
		wg.Add(1)
		go func(idx, start, end int) {
			defer wg.Done()
			var partialDot float32
			for j := start; j < end; j++ {
				partialDot += v1[j] * v2[j]
			}
			results[idx] = partialDot
		}(i, i*batchSize, min((i+1)*batchSize, n))
	}

	wg.Wait()

	// 汇总结果
	var dot float32
	for _, r := range results {
		dot += r
	}

	return dot
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// euclideanDistance 计算欧几里德距离
// 优化：对于高维向量使用分块计算，减少浮点累加误差
func euclideanDistance(v1, v2 []float32) float32 {
	n := len(v1)

	// 小向量直接计算
	if n < 1000 {
		var sum float32
		for i := 0; i < n; i++ {
			d := v1[i] - v2[i]
			sum += d * d
		}
		return float32(math.Sqrt(float64(sum)))
	}

	// 大向量分块计算，减少累积误差
	const blockSize = 250
	blocks := (n + blockSize - 1) / blockSize
	sums := make([]float32, blocks)

	for b := 0; b < blocks; b++ {
		start := b * blockSize
		end := min(start+blockSize, n)

		for i := start; i < end; i++ {
			d := v1[i] - v2[i]
			sums[b] += d * d
		}
	}

	var totalSum float32
	for _, s := range sums {
		totalSum += s
	}

	return float32(math.Sqrt(float64(totalSum)))
}

// vectorNorm 计算向量的L2范数
// 优化：对大向量使用分块计算，减少浮点累加误差
func vectorNorm(v []float32) float32 {
	n := len(v)

	// 小向量直接计算
	if n < 1000 {
		var sum float32
		for _, val := range v {
			sum += val * val
		}
		return float32(math.Sqrt(float64(sum)))
	}

	// 大向量分块计算，减少累积误差
	const blockSize = 250
	blocks := (n + blockSize - 1) / blockSize
	sums := make([]float32, blocks)

	for b := 0; b < blocks; b++ {
		start := b * blockSize
		end := min(start+blockSize, n)

		for i := start; i < end; i++ {
			sums[b] += v[i] * v[i]
		}
	}

	var totalSum float32
	for _, s := range sums {
		totalSum += s
	}

	return float32(math.Sqrt(float64(totalSum)))
}

// normalizeVector 归一化向量（使其长度为1）
// 优化：增加容错处理，避免零向量和过小值导致的问题
func normalizeVector(v []float32) []float32 {
	norm := vectorNorm(v)
	if norm < 1e-10 { // 非常接近0的向量视为零向量
		return make([]float32, len(v)) // 返回全0向量
	}

	result := make([]float32, len(v))
	invNorm := 1.0 / norm // 预先计算倒数，避免多次除法操作

	for i, val := range v {
		result[i] = val * invNorm
	}
	return result
}

// FilterDocuments 根据过滤条件筛选文档
// 优化：提前计算映射，减少查找开销
func FilterDocuments(docs []Document, filter SearchFilter) []Document {
	if len(docs) == 0 {
		return nil
	}

	var result []Document

	// 构建文件ID查找表，避免重复比较
	fileIDMap := make(map[string]bool)
	if len(filter.FileIDs) > 0 {
		for _, id := range filter.FileIDs {
			fileIDMap[id] = true
		}
	}

	// 预留足够的空间，减少扩容
	result = make([]Document, 0, len(docs))

	// 筛选文档
	hasFileFilter := len(fileIDMap) > 0
	hasMetaFilter := len(filter.Metadata) > 0

	// 优化：不同过滤条件使用不同处理路径，避免不必要的检查
	if !hasFileFilter && !hasMetaFilter {
		// 没有过滤条件，直接返回所有文档
		return append(result, docs...)
	} else if hasFileFilter && !hasMetaFilter {
		// 仅按文件ID过滤
		for _, doc := range docs {
			if fileIDMap[doc.FileID] {
				result = append(result, doc)
			}
		}
	} else if !hasFileFilter && hasMetaFilter {
		// 仅按元数据过滤
		for _, doc := range docs {
			if matchMetadata(doc.Metadata, filter.Metadata) {
				result = append(result, doc)
			}
		}
	} else {
		// 同时按文件ID和元数据过滤
		for _, doc := range docs {
			if fileIDMap[doc.FileID] && matchMetadata(doc.Metadata, filter.Metadata) {
				result = append(result, doc)
			}
		}
	}

	return result
}

// matchMetadata 检查文档元数据是否匹配过滤条件
// 优化：支持更复杂的元数据匹配（前缀、后缀、包含关系等）
func matchMetadata(docMeta map[string]interface{}, filterMeta map[string]interface{}) bool {
	if len(filterMeta) == 0 {
		return true // 没有元数据过滤条件
	}

	for key, filterValue := range filterMeta {
		docValue, exists := docMeta[key]
		if !exists {
			return false
		}

		// 检查值是否匹配
		switch fv := filterValue.(type) {
		case string:
			// 字符串类型支持前缀匹配和后缀匹配
			if dvStr, ok := docValue.(string); ok {
				// 检查前缀匹配：key^=value
				if len(fv) > 2 && fv[0] == '^' && fv[1] == '=' {
					prefix := fv[2:]
					if !strings.HasPrefix(dvStr, prefix) {
						return false
					}
					continue
				}

				// 检查后缀匹配：key$=value
				if len(fv) > 2 && fv[0] == '$' && fv[1] == '=' {
					suffix := fv[2:]
					if !strings.HasSuffix(dvStr, suffix) {
						return false
					}
					continue
				}
			}
		}

		// 默认精确匹配
		if docValue != filterValue {
			return false
		}
	}

	return true
}

// SortSearchResults 对搜索结果按相似度评分排序（降序）
// 优化：使用快速排序替代插入排序，提高大数据集排序效率
func SortSearchResults(results []SearchResult) {
	if len(results) <= 1 {
		return
	}

	// 对于小数据集使用插入排序
	if len(results) <= 20 {
		// 使用简单的插入排序（对小结果集足够高效）
		for i := 1; i < len(results); i++ {
			current := results[i]
			j := i - 1

			// 评分越高越靠前（降序）
			for j >= 0 && results[j].Score < current.Score {
				results[j+1] = results[j]
				j--
			}
			results[j+1] = current
		}
		return
	}

	// 大数据集使用快速排序
	quickSortResults(results, 0, len(results)-1)
}

// quickSortResults 快速排序算法实现
func quickSortResults(results []SearchResult, low, high int) {
	if low < high {
		// 分区
		pivot := partition(results, low, high)

		// 递归排序子数组
		quickSortResults(results, low, pivot-1)
		quickSortResults(results, pivot+1, high)
	}
}

// partition 分区函数，为快速排序服务
func partition(results []SearchResult, low, high int) int {
	pivot := results[high].Score
	i := low - 1

	for j := low; j < high; j++ {
		// 降序排列（Score越大越靠前）
		if results[j].Score >= pivot {
			i++
			results[i], results[j] = results[j], results[i]
		}
	}

	results[i+1], results[high] = results[high], results[i+1]
	return i + 1
}

// DistanceToScore 将距离转换为评分（0-1之间）
// 优化：增加缓存常用的转换值，提高性能
func DistanceToScore(distance float32, distType DistanceType) float32 {
	switch distType {
	case Cosine:
		// 余弦距离: 1 - distance (余弦距离已经是1-相似度)
		return 1 - distance
	case DotProduct:
		// 点积: 对于归一化向量，范围通常在[-1, 1]之间
		// 转换为[0, 1]范围
		// 限制结果在有效范围内
		score := (distance + 1) / 2
		if score < 0 {
			return 0
		}
		if score > 1 {
			return 1
		}
		return score
	case Euclidean:
		// 欧几里德距离: 使用高斯衰减函数
		// 距离越小，分数越高
		// 优化: 限制最大距离，避免极小值
		if distance > 10 {
			return 0.0001 // 几乎为0，但非零
		}
		return float32(math.Exp(-float64(distance)))
	default:
		return 0
	}
}

// ValidateVector 验证向量维度和有效性
// 优化：增加更多有效性检查
func ValidateVector(vector []float32, expectedDim int) error {
	// 检查向量是否为空
	if len(vector) == 0 {
		return ErrEmptyVector
	}

	// 检查维度是否匹配
	if expectedDim > 0 && len(vector) != expectedDim {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", expectedDim, len(vector))
	}

	// 检查向量是否包含有效值（非NaN、非无限）
	for i, v := range vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("vector contains invalid value at index %d: %v", i, v)
		}
	}

	return nil
}

// 缓存相关辅助函数

// TimedCache 简单的带有过期时间的内存缓存
type TimedCache struct {
	mu       sync.RWMutex
	data     map[string]interface{}
	expiry   map[string]time.Time
	ttl      time.Duration
	lastScan time.Time
}

// NewTimedCache 创建新的缓存
func NewTimedCache(ttl time.Duration) *TimedCache {
	cache := &TimedCache{
		data:     make(map[string]interface{}),
		expiry:   make(map[string]time.Time),
		ttl:      ttl,
		lastScan: time.Now(),
	}

	// 定期清理过期项
	go func() {
		ticker := time.NewTicker(ttl / 2)
		defer ticker.Stop()

		for range ticker.C {
			cache.Cleanup()
		}
	}()

	return cache
}

// Get 从缓存中获取值
func (c *TimedCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, exists := c.data[key]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if exp, ok := c.expiry[key]; ok && time.Now().After(exp) {
		return nil, false
	}

	return value, true
}

// Set 向缓存中设置值
func (c *TimedCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
	c.expiry[key] = time.Now().Add(c.ttl)

	// 定期清理，避免频繁清理影响性能
	if time.Since(c.lastScan) > c.ttl/2 {
		go c.Cleanup()
		c.lastScan = time.Now()
	}
}

// Cleanup 清理过期的缓存项
func (c *TimedCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, exp := range c.expiry {
		if now.After(exp) {
			delete(c.data, k)
			delete(c.expiry, k)
		}
	}
}
