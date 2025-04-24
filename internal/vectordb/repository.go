package vectordb

import (
	"fmt"
	"math"
	"sync"
)

// BaseRepository 基础仓库实现
// 为各种具体的向量数据库实现提供共享功能
type BaseRepository struct {
	mu        sync.RWMutex        // 用于并发安全的互斥锁
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
	}

	return 1.0 - similarity
}

// dotProduct 计算两个向量的点积
func dotProduct(v1, v2 []float32) float32 {
	var dot float32
	for i := 0; i < len(v1); i++ {
		dot += v1[i] * v2[i]
	}
	return dot
}

// euclideanDistance 计算欧几里德距离
func euclideanDistance(v1, v2 []float32) float32 {
	var sum float32
	for i := 0; i < len(v1); i++ {
		d := v1[i] - v2[i]
		sum += d * d
	}
	return float32(math.Sqrt(float64(sum)))
}

// vectorNorm 计算向量的L2范数
func vectorNorm(v []float32) float32 {
	var sum float32
	for _, val := range v {
		sum += val * val
	}
	return float32(math.Sqrt(float64(sum)))
}

// normalizeVector 归一化向量（使其长度为1）
func normalizeVector(v []float32) []float32 {
	norm := vectorNorm(v)
	if norm == 0 {
		return v // 零向量无法归一化
	}

	result := make([]float32, len(v))
	for i, val := range v {
		result[i] = val / norm
	}
	return result
}

// FilterDocuments 根据过滤条件筛选文档
func FilterDocuments(docs []Document, filter SearchFilter) []Document {
	if len(docs) == 0 {
		return nil
	}

	var result []Document

	// 文件ID过滤
	fileIDMap := make(map[string]bool)
	if len(filter.FileIDs) > 0 {
		for _, id := range filter.FileIDs {
			fileIDMap[id] = true
		}
	}

	for _, doc := range docs {
		// 如果指定了文件ID过滤，检查当前文档是否匹配
		if len(fileIDMap) > 0 && !fileIDMap[doc.FileID] {
			continue
		}

		// 元数据过滤
		if !matchMetadata(doc.Metadata, filter.Metadata) {
			continue
		}

		result = append(result, doc)
	}

	return result
}

// matchMetadata 检查文档元数据是否匹配过滤条件
func matchMetadata(docMeta map[string]interface{}, filterMeta map[string]interface{}) bool {
	if len(filterMeta) == 0 {
		return true // 没有元数据过滤条件
	}

	for key, filterValue := range filterMeta {
		docValue, exists := docMeta[key]
		if !exists || docValue != filterValue {
			return false
		}
	}

	return true
}

// SortSearchResults 对搜索结果按相似度评分排序（降序）
func SortSearchResults(results []SearchResult) {
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
}

// DistanceToScore 将距离转换为评分（0-1之间）
// 不同距离度量需要不同的转换方法
func DistanceToScore(distance float32, distType DistanceType) float32 {
	switch distType {
	case Cosine:
		// 余弦距离: 1 - distance (余弦距离已经是1-相似度)
		return 1 - distance
	case DotProduct:
		// 点积: 对于归一化向量，范围通常在[-1, 1]之间
		// 转换为[0, 1]范围
		return (distance + 1) / 2
	case Euclidean:
		// 欧几里德距离: 使用高斯衰减函数
		// 距离越小，分数越高
		return float32(math.Exp(-float64(distance)))
	default:
		return 0
	}
}

// ValidateVector 验证向量维度和有效性
func ValidateVector(vector []float32, expectedDim int) error {
	if len(vector) == 0 {
		return ErrEmptyVector
	}

	if expectedDim > 0 && len(vector) != expectedDim {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", expectedDim, len(vector))
	}

	return nil
}
