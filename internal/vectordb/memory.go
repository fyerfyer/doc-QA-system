package vectordb

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryRepository 内存向量仓库实现
// 用于开发和测试环境的简单内存存储
type MemoryRepository struct {
	*BaseRepository                     // 嵌入基础仓库实现
	mu              sync.RWMutex        // 读写锁，确保并发安全
	documents       map[string]Document // 文档存储，ID到文档的映射
	fileToDocIDs    map[string][]string // 文件ID到文档ID的映射
}

// NewMemoryRepository 创建内存向量仓库
func NewMemoryRepository(config Config) (Repository, error) {
	// 确保维度大于0
	if config.Dimension <= 0 {
		return nil, fmt.Errorf("vector dimension must be positive")
	}

	// 确保距离类型有效
	distType := DistanceType(config.DistanceType)
	if distType != Cosine && distType != DotProduct && distType != Euclidean {
		distType = Cosine // 默认使用余弦距离
	}

	// 创建基础仓库
	base := NewBaseRepository(config.Dimension, distType)

	// 创建并返回内存仓库
	return &MemoryRepository{
		BaseRepository: base,
		documents:      make(map[string]Document),
		fileToDocIDs:   make(map[string][]string),
	}, nil
}

// Add 添加单个文档到内存仓库
func (r *MemoryRepository) Add(doc Document) error {
	// 验证向量维度
	if err := ValidateVector(doc.Vector, r.dimension); err != nil {
		return err
	}

	// 如果没有设置创建时间，设置为当前时间
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}

	// 如果没有初始化元数据，则创建一个空映射
	if doc.Metadata == nil {
		doc.Metadata = make(map[string]interface{})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 存储文档
	r.documents[doc.ID] = doc

	// 更新文件到文档的映射
	r.fileToDocIDs[doc.FileID] = append(r.fileToDocIDs[doc.FileID], doc.ID)

	return nil
}

// AddBatch 批量添加文档到内存仓库
func (r *MemoryRepository) AddBatch(docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 使用单个锁进行批处理，避免多次加解锁开销
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range docs {
		doc := &docs[i] // 使用指针避免复制

		// 验证向量维度
		if err := ValidateVector(doc.Vector, r.dimension); err != nil {
			return fmt.Errorf("invalid vector for document %s: %v", doc.ID, err)
		}

		// 设置创建时间（如果未设置）
		if doc.CreatedAt.IsZero() {
			doc.CreatedAt = time.Now()
		}

		// 初始化元数据（如果未设置）
		if doc.Metadata == nil {
			doc.Metadata = make(map[string]interface{})
		}

		// 存储文档
		r.documents[doc.ID] = *doc

		// 更新文件到文档的映射
		r.fileToDocIDs[doc.FileID] = append(r.fileToDocIDs[doc.FileID], doc.ID)
	}

	return nil
}

// Get 获取单个文档
func (r *MemoryRepository) Get(id string) (Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	doc, exists := r.documents[id]
	if !exists {
		return Document{}, ErrDocumentNotFound
	}

	return doc, nil
}

// Delete 删除单个文档
func (r *MemoryRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, exists := r.documents[id]
	if !exists {
		return ErrDocumentNotFound
	}

	// 删除文档
	delete(r.documents, id)

	// 更新文件到文档映射
	if fileIDs, ok := r.fileToDocIDs[doc.FileID]; ok {
		updatedIDs := make([]string, 0, len(fileIDs)-1)
		for _, docID := range fileIDs {
			if docID != id {
				updatedIDs = append(updatedIDs, docID)
			}
		}

		if len(updatedIDs) == 0 {
			delete(r.fileToDocIDs, doc.FileID)
		} else {
			r.fileToDocIDs[doc.FileID] = updatedIDs
		}
	}

	return nil
}

// DeleteByFileID 删除指定文件的所有段落
func (r *MemoryRepository) DeleteByFileID(fileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 获取属于该文件的所有文档ID
	docIDs, exists := r.fileToDocIDs[fileID]
	if !exists {
		// 如果没有找到文件ID，不需要执行任何操作
		return nil
	}

	// 删除所有关联的文档
	for _, id := range docIDs {
		delete(r.documents, id)
	}

	// 删除文件到文档的映射
	delete(r.fileToDocIDs, fileID)

	return nil
}

// Search 相似度搜索
func (r *MemoryRepository) Search(vector []float32, filter SearchFilter) ([]SearchResult, error) {
	// 验证向量
	if err := ValidateVector(vector, r.dimension); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 过滤文档
	var filteredDocs []Document
	if len(filter.FileIDs) > 0 {
		// 如果指定了文件ID，只检索这些文件的文档
		fileIDMap := make(map[string]bool)
		for _, id := range filter.FileIDs {
			fileIDMap[id] = true
		}

		for _, doc := range r.documents {
			if fileIDMap[doc.FileID] {
				filteredDocs = append(filteredDocs, doc)
			}
		}
	} else {
		// 否则检索所有文档
		filteredDocs = make([]Document, 0, len(r.documents))
		for _, doc := range r.documents {
			filteredDocs = append(filteredDocs, doc)
		}
	}

	// 进一步基于元数据过滤
	filteredDocs = FilterDocuments(filteredDocs, filter)

	// 计算距离并创建结果
	results := make([]SearchResult, 0, len(filteredDocs))
	for _, doc := range filteredDocs {
		dist, err := ComputeDistance(vector, doc.Vector, r.distType)
		if err != nil {
			return nil, fmt.Errorf("error computing distance: %v", err)
		}

		// 计算得分，转换取决于距离类型
		score := DistanceToScore(dist, r.distType)

		// 只保留高于最小分数的结果
		if score >= filter.MinScore {
			results = append(results, SearchResult{
				Document: doc,
				Score:    score,
				Distance: dist,
			})
		}
	}

	// 按得分排序（从高到低）
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 只返回前N个结果
	if filter.MaxResults > 0 && len(results) > filter.MaxResults {
		results = results[:filter.MaxResults]
	}

	return results, nil
}

// Count 获取文档总数
func (r *MemoryRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.documents), nil
}

// Close 关闭数据库连接
// 对于内存实现这是一个空操作
func (r *MemoryRepository) Close() error {
	return nil
}

// 在包初始化时注册内存仓库
func init() {
	RegisterRepository("memory", NewMemoryRepository)
}
