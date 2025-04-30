package vectordb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataIntelligenceCrew/go-faiss"
)

// FaissRepository 实现基于Faiss的向量仓库
type FaissRepository struct {
	*BaseRepository
	mu             sync.RWMutex        // 并发锁
	index          faiss.Index         // Faiss索引
	documents      map[string]Document // 文档存储
	fileToDocIDs   map[string][]string // 文件ID到文档ID的映射
	idToPosition   map[string]int      // 文档ID到向量位置的映射
	indexPath      string              // 索引文件路径
	metaPath       string              // 元数据文件路径
	dimension      int                 // 向量维度
	distanceType   DistanceType        // 距离计算类型
	saveOnClose    bool                // 关闭时是否保存
	autoSave       bool                // 是否自动保存
	autoSaveCount  int                 // 自动保存的操作计数阈值
	operationCount int                 // 当前操作计数
	queryCache     *TimedCache         // 查询缓存
	lastSave       time.Time           // 上次保存时间
}

// NewFaissRepository 创建新的Faiss向量仓库
func NewFaissRepository(config Config) (Repository, error) {
	// 检查向量维度
	if config.Dimension <= 0 {
		return nil, fmt.Errorf("vector dimension must be positive")
	}

	// 确保目录存在（如果指定了路径）
	if config.Path != "" && !config.InMemory {
		if err := os.MkdirAll(filepath.Dir(config.Path), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for index: %v", err)
		}
	}

	// 确保距离类型有效
	distType := config.DistanceType
	if distType == "" {
		distType = Cosine // 默认使用余弦距离
	}

	// 创建基础仓库
	base := NewBaseRepository(config.Dimension, distType)

	// 设置文件路径
	indexPath := config.Path
	metaPath := ""
	if indexPath != "" {
		metaPath = indexPath + ".meta.json"
	}

	// 创建仓库实例
	repo := &FaissRepository{
		BaseRepository: base,
		documents:      make(map[string]Document),
		fileToDocIDs:   make(map[string][]string),
		idToPosition:   make(map[string]int),
		indexPath:      indexPath,
		metaPath:       metaPath,
		dimension:      config.Dimension,
		distanceType:   distType,
		saveOnClose:    true,
		autoSave:       true,
		autoSaveCount:  100,                            // 默认每100次操作自动保存一次
		queryCache:     NewTimedCache(5 * time.Minute), // 查询缓存5分钟
		lastSave:       time.Now(),
	}

	var index faiss.Index
	var err error

	// 尝试从文件加载索引
	if indexPath != "" && !config.InMemory && fileExists(indexPath) {
		// 加载预先存储的索引文件
		index, err = faiss.ReadIndex(indexPath, 0)
		if err != nil {
			// 如果加载失败但允许创建，则创建新索引
			if config.CreateIfNotExists {
				index, err = createFaissIndex(config.Dimension, distType)
				if err != nil {
					return nil, fmt.Errorf("failed to create Faiss index: %v", err)
				}
			} else {
				return nil, fmt.Errorf("failed to load Faiss index: %v", err)
			}
		} else {
			// 加载元数据
			if err := repo.loadMetadata(metaPath); err != nil {
				// 元数据加载失败只记录警告，不阻止继续
				fmt.Printf("Warning: Failed to load metadata: %v\n", err)
			}
		}
	} else {
		// 创建新索引
		index, err = createFaissIndex(config.Dimension, distType)
		if err != nil {
			return nil, fmt.Errorf("failed to create Faiss index: %v", err)
		}
	}

	repo.index = index

	return repo, nil
}

// 创建Faiss索引时根据不同的向量维度和数据量动态选择索引类型
func createFaissIndex(dimension int, distType DistanceType) (faiss.Index, error) {
	var metric int

	// 根据距离类型选择合适的度量方式
	switch distType {
	case Cosine, DotProduct:
		// 对于余弦距离和点积，使用内积度量方式
		metric = int(faiss.MetricInnerProduct)
	case Euclidean:
		// 对于欧几里得距离，使用L2度量方式
		metric = int(faiss.MetricL2)
	default:
		// 默认使用L2度量方式
		metric = int(faiss.MetricL2)
	}

	// 创建扁平索引（最简单但速度适中的索引类型）
	return faiss.NewIndexFlat(dimension, metric)
}

// Add 添加单个文档到仓库
func (r *FaissRepository) Add(doc Document) error {
	// 验证向量
	if err := ValidateVector(doc.Vector, r.dimension); err != nil {
		return err
	}

	// 对余弦距离执行向量归一化
	if r.distanceType == Cosine {
		doc.Vector = normalizeVector(doc.Vector)
	}

	// 设置创建时间（如果未设置）
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}

	// 初始化元数据（如果未设置）
	if doc.Metadata == nil {
		doc.Metadata = make(map[string]interface{})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 获取当前向量总数作为新向量的位置
	nextPos := int(r.index.Ntotal())

	// 添加向量到Faiss索引
	err := r.index.Add(doc.Vector)
	if err != nil {
		return fmt.Errorf("failed to add vector to Faiss index: %v", err)
	}

	// 更新映射关系
	r.documents[doc.ID] = doc
	r.idToPosition[doc.ID] = nextPos
	r.fileToDocIDs[doc.FileID] = append(r.fileToDocIDs[doc.FileID], doc.ID)
	r.operationCount++

	// 如果启用了自动保存，检查是否需要保存
	if r.autoSave && r.shouldSave() {
		if err := r.saveIndex(); err != nil {
			// 保存失败只记录错误，不影响添加操作
			fmt.Printf("Warning: Failed to auto save index: %v\n", err)
		}
		r.operationCount = 0
		r.lastSave = time.Now()
	}

	return nil
}

// shouldSave 确定是否需要保存索引
func (r *FaissRepository) shouldSave() bool {
	// 操作次数超过阈值或上次保存时间超过1小时
	return r.operationCount >= r.autoSaveCount || time.Since(r.lastSave) > time.Hour
}

// AddBatch 批量添加文档到仓库
func (r *FaissRepository) AddBatch(docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 预处理所有向量
	vectors := make([][]float32, len(docs))
	for i := range docs {
		// 验证向量
		if err := ValidateVector(docs[i].Vector, r.dimension); err != nil {
			return fmt.Errorf("invalid vector in document %s: %v", docs[i].ID, err)
		}

		// 对余弦距离执行向量归一化
		if r.distanceType == Cosine {
			docs[i].Vector = normalizeVector(docs[i].Vector)
		}

		vectors[i] = docs[i].Vector

		// 设置创建时间（如果未设置）
		if docs[i].CreatedAt.IsZero() {
			docs[i].CreatedAt = time.Now()
		}

		// 初始化元数据（如果未设置）
		if docs[i].Metadata == nil {
			docs[i].Metadata = make(map[string]interface{})
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 记录起始位置
	startPos := int(r.index.Ntotal())

	// 使用循环添加向量
	for i, vector := range vectors {
		if err := r.index.Add(vector); err != nil {
			return fmt.Errorf("failed to add vector %d to Faiss index: %v", i, err)
		}
	}

	// 更新映射关系
	for i, doc := range docs {
		position := startPos + i
		r.documents[doc.ID] = doc
		r.idToPosition[doc.ID] = position
		r.fileToDocIDs[doc.FileID] = append(r.fileToDocIDs[doc.FileID], doc.ID)
	}

	r.operationCount += len(docs)

	// 如果启用了自动保存，检查是否需要保存
	if r.autoSave && r.shouldSave() {
		if err := r.saveIndex(); err != nil {
			fmt.Printf("Warning: Failed to auto save index: %v\n", err)
		}
		r.operationCount = 0
		r.lastSave = time.Now()
	}

	return nil
}

// Get 获取单个文档
func (r *FaissRepository) Get(id string) (Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	doc, exists := r.documents[id]
	if !exists {
		return Document{}, ErrDocumentNotFound
	}

	return doc, nil
}

// Delete 删除单个文档
func (r *FaissRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 获取文档
	doc, exists := r.documents[id]
	if !exists {
		return ErrDocumentNotFound
	}

	// 在内存中清除对应映射
	delete(r.documents, id)
	delete(r.idToPosition, id)

	// 更新文件ID到文档ID的映射
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

	// 记录操作
	r.operationCount++

	// 清除与该文档相关的查询缓存
	r.clearRelatedCaches(doc.FileID)

	return nil
}

// DeleteByFileID 删除指定文件的所有文档
func (r *FaissRepository) DeleteByFileID(fileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 获取文件相关的所有文档ID
	docIDs, exists := r.fileToDocIDs[fileID]
	if !exists {
		// 如果没有找到文件ID，不需要执行任何操作
		return nil
	}

	// 删除所有关联的文档记录
	for _, id := range docIDs {
		delete(r.documents, id)
		delete(r.idToPosition, id)
	}

	// 删除文件映射
	delete(r.fileToDocIDs, fileID)
	r.operationCount += len(docIDs)

	// 清除与该文件相关的查询缓存
	r.clearRelatedCaches(fileID)

	return nil
}

// clearRelatedCaches 清除与特定文件相关的查询缓存
func (r *FaissRepository) clearRelatedCaches(fileID string) {
	// 简单实现：清空整个缓存
	r.queryCache = NewTimedCache(5 * time.Minute)
}

// Search 相似度搜索
func (r *FaissRepository) Search(vector []float32, filter SearchFilter) ([]SearchResult, error) {
	// 验证查询向量
	if err := ValidateVector(vector, r.dimension); err != nil {
		return nil, err
	}

	// 对于余弦距离，需要对查询向量进行归一化
	if r.distanceType == Cosine {
		vector = normalizeVector(vector)
	}

	// 基于向量和过滤器生成缓存键
	cacheKey := generateCacheKey(vector, filter)
	// fmt.Printf("generate cache key: %s\n", cacheKey)

	// 尝试从缓存获取结果
	if cachedValue, found := r.queryCache.Get(cacheKey); found {
		// fmt.Println("cache hit!")
		if results, ok := cachedValue.([]SearchResult); ok {
			// 检查缓存的元数据
			if len(results) > 0 {
				fmt.Printf("cached document ID: %s, metadata: %+v\n", results[0].Document.ID, results[0].Document.Metadata)
			}
			return results, nil
		}
		// fmt.Println("failed to cast cached value to SearchResult slice")
	} else {
		// fmt.Println("cache miss!")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 如果没有文档，直接返回空结果
	if len(r.documents) == 0 {
		return []SearchResult{}, nil
	}

	// 确定要检索的向量数量
	k := filter.MaxResults
	if k <= 0 {
		k = 10 // 默认返回前10个结果
	}

	// 查询更多结果以确保过滤后有足够的结果
	queryLimit := k * 4
	total := int(r.index.Ntotal())
	if queryLimit > total {
		queryLimit = total
	}
	if queryLimit == 0 {
		return []SearchResult{}, nil
	}

	// 使用Faiss执行搜索，获取距离和索引
	distances, indices, err := r.index.Search(vector, int64(queryLimit))
	if err != nil {
		return nil, fmt.Errorf("failed to search Faiss index: %v", err)
	}

	// 处理搜索结果
	results, err := r.processSearchResults(distances, indices, filter)
	if err != nil {
		return nil, err
	}

	// 缓存结果 - 关键修改：存入深拷贝而不是原引用
	r.queryCache.Set(cacheKey, deepCopyResults(results))

	return results, nil
}

// processSearchResults 处理Faiss返回的搜索结果
func (r *FaissRepository) processSearchResults(
	distances []float32,
	indices []int64,
	filter SearchFilter,
) ([]SearchResult, error) {
	var results []SearchResult

	// 预分配一个足够大的切片以减少重新分配
	results = make([]SearchResult, 0, len(indices))

	// 过滤条件
	hasFileFilter := len(filter.FileIDs) > 0
	hasMetaFilter := len(filter.Metadata) > 0

	// 文件ID过滤器的快速查找表
	fileFilter := make(map[string]bool)
	if hasFileFilter {
		for _, id := range filter.FileIDs {
			fileFilter[id] = true
		}
	}

	// 遍历搜索结果
	for i := 0; i < len(indices); i++ {
		idx := indices[i]
		if idx < 0 {
			continue // 跳过无效索引
		}

		// 查找索引对应的文档ID
		var docID string
		found := false

		for id, pos := range r.idToPosition {
			if pos == int(idx) {
				docID = id
				found = true
				break
			}
		}

		if !found {
			continue // 跳过无法找到文档ID的结果
		}

		// 获取文档对象
		doc, exists := r.documents[docID]
		if !exists {
			continue // 跳过不存在的文档
		}

		// 应用文件ID过滤器
		if hasFileFilter && !fileFilter[doc.FileID] {
			continue
		}

		// 应用元数据过滤器
		if hasMetaFilter && !matchMetadata(doc.Metadata, filter.Metadata) {
			continue
		}

		// 处理距离值
		dist := distances[i]

		// 对于余弦相似度或点积，FAISS使用内积（越大越相似）
		// 需要将其转换为距离（越小越相似）
		if r.distanceType == Cosine || r.distanceType == DotProduct {
			// 内积范围在[-1,1]，余弦距离 = 1 - 内积
			dist = 1 - dist
		}

		// 根据距离计算得分
		score := DistanceToScore(dist, r.distanceType)

		// 仅保留高于最低得分阈值的结果
		if score < filter.MinScore {
			continue
		}

		// 添加到结果集
		results = append(results, SearchResult{
			Document: doc,
			Score:    score,
			Distance: dist,
		})

		// 如果已经收集足够的结果，提前退出
		if len(results) >= filter.MaxResults && filter.MaxResults > 0 {
			break
		}
	}

	// 对结果按分数排序
	SortSearchResults(results)

	// 如果有最大结果数限制，截取前N个
	if filter.MaxResults > 0 && len(results) > filter.MaxResults {
		results = results[:filter.MaxResults]
	}

	// 打印结果
	// if len(results) > 0 {
	// 	fmt.Printf("Handling documents with ID: %s, metadata: %+v\n", results[0].Document.ID, results[0].Document.Metadata)
	// }

	return results, nil
}

// generateCacheKey 为搜索查询生成缓存键
func generateCacheKey(vector []float32, filter SearchFilter) string {
	// 简化实现：使用向量的前几个值和长度作为缓存键的一部分
	key := fmt.Sprintf("v%d_%f_%f", len(vector), vector[0], vector[1])

	// 添加过滤条件信息
	if len(filter.FileIDs) > 0 {
		for _, fileID := range filter.FileIDs {
			key += "_f" + fileID[:min(8, len(fileID))]
		}
	}

	return key
}

// 添加一个新的辅助函数来创建结果的深拷贝
func deepCopyResults(results []SearchResult) []SearchResult {
	copied := make([]SearchResult, len(results))
	for i, result := range results {
		copied[i] = SearchResult{
			Score:    result.Score,
			Distance: result.Distance,
			Document: deepCopyDocument(result.Document),
		}
	}
	return copied
}

// 深拷贝Document对象
func deepCopyDocument(doc Document) Document {
	copiedDoc := doc

	// 拷贝向量
	if doc.Vector != nil {
		copiedDoc.Vector = make([]float32, len(doc.Vector))
		copy(copiedDoc.Vector, doc.Vector)
	}

	// 拷贝元数据
	if doc.Metadata != nil {
		copiedDoc.Metadata = make(map[string]interface{})
		for k, v := range doc.Metadata {
			copiedDoc.Metadata[k] = v
		}
	}

	return copiedDoc
}

// Count 获取文档总数
func (r *FaissRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.documents), nil
}

// Close 关闭仓库
func (r *FaissRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 如果设置了保存标志且有索引路径，保存索引和元数据
	if r.saveOnClose && r.indexPath != "" {
		if err := r.saveIndex(); err != nil {
			return fmt.Errorf("failed to save index on close: %v", err)
		}
	}

	return nil
}

// saveIndex 保存索引和文档数据到文件
func (r *FaissRepository) saveIndex() error {
	// 如果没有指定索引路径，不执行保存
	if r.indexPath == "" {
		return nil
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(r.indexPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// 保存Faiss索引
	if err := faiss.WriteIndex(r.index, r.indexPath); err != nil {
		return fmt.Errorf("failed to write Faiss index: %v", err)
	}

	// 保存元数据
	return r.saveMetadata()
}

// saveMetadata 保存文档元数据到文件
func (r *FaissRepository) saveMetadata() error {
	// 如果没有指定元数据路径，不执行保存
	if r.metaPath == "" {
		return nil
	}

	// 准备元数据结构
	metadata := struct {
		Documents      map[string]Document `json:"documents"`
		FileToDocIDs   map[string][]string `json:"file_to_doc_ids"`
		IDToPosition   map[string]int      `json:"id_to_position"`
		OperationCount int                 `json:"operation_count"`
	}{
		Documents:      r.documents,
		FileToDocIDs:   r.fileToDocIDs,
		IDToPosition:   r.idToPosition,
		OperationCount: r.operationCount,
	}

	// 序列化为JSON
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	// 写入文件
	if err := os.WriteFile(r.metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %v", err)
	}

	return nil
}

// loadMetadata 从文件加载文档元数据
func (r *FaissRepository) loadMetadata(path string) error {
	// 如果没有指定路径或文件不存在，不执行加载
	if path == "" || !fileExists(path) {
		return nil
	}

	// 读取文件
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read metadata file: %v", err)
	}

	// 准备元数据结构
	metadata := struct {
		Documents      map[string]Document `json:"documents"`
		FileToDocIDs   map[string][]string `json:"file_to_doc_ids"`
		IDToPosition   map[string]int      `json:"id_to_position"`
		OperationCount int                 `json:"operation_count"`
	}{}

	// 解析JSON
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %v", err)
	}

	// 应用加载的元数据
	r.documents = metadata.Documents
	r.fileToDocIDs = metadata.FileToDocIDs
	r.idToPosition = metadata.IDToPosition
	r.operationCount = metadata.OperationCount

	return nil
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// 在包初始化时注册Faiss仓库
func init() {
	RegisterRepository("faiss", NewFaissRepository)
}
