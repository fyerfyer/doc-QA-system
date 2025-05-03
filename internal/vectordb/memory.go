package vectordb

import (
	"fmt"
	"runtime"
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
	vectorCache     *vectorCache        // 向量缓存，用于加速常见搜索
}

// vectorCache 用于缓存已计算的向量距离和查询结果
type vectorCache struct {
	mu              sync.RWMutex                  // 缓存访问锁
	distCache       map[string]map[string]float32 // 向量距离缓存 [v1_id][v2_id] -> distance
	mostRecent      []string                      // 最近使用的查询向量标识
	queryResults    map[string][]SearchResult     // 查询结果缓存
	maxCacheEntries int                           // 最大缓存条目数
	maxResultsAge   time.Duration                 // 结果缓存有效期
	resultsAge      map[string]time.Time          // 结果缓存创建时间
}

// newVectorCache 创建新的向量缓存
func newVectorCache() *vectorCache {
	return &vectorCache{
		distCache:       make(map[string]map[string]float32),
		mostRecent:      make([]string, 0, 100),
		queryResults:    make(map[string][]SearchResult),
		resultsAge:      make(map[string]time.Time),
		maxCacheEntries: 1000,             // 默认最多缓存1000个查询结果
		maxResultsAge:   time.Minute * 10, // 默认缓存10分钟
	}
}

// cacheKey 为查询生成缓存键
func cacheKey(vector []float32, filter SearchFilter) string {
	// 简单使用向量的长度和前几个值作为缓存键的一部分
	key := fmt.Sprintf("v%d_%f_%f", len(vector), vector[0], vector[1])

	// 添加过滤条件信息
	if len(filter.FileIDs) > 0 {
		key += fmt.Sprintf("_f%d", len(filter.FileIDs))
	}
	if len(filter.Metadata) > 0 {
		key += fmt.Sprintf("_m%d", len(filter.Metadata))
	}
	key += fmt.Sprintf("_r%d", filter.MaxResults)

	return key
}

// addDistCache 添加向量距离到缓存
func (c *vectorCache) addDistCache(v1ID, v2ID string, dist float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 确保内部映射存在
	if _, ok := c.distCache[v1ID]; !ok {
		c.distCache[v1ID] = make(map[string]float32)
	}
	c.distCache[v1ID][v2ID] = dist

	// 对称性：d(v1,v2) = d(v2,v1)
	if _, ok := c.distCache[v2ID]; !ok {
		c.distCache[v2ID] = make(map[string]float32)
	}
	c.distCache[v2ID][v1ID] = dist

	// 维护最近使用列表
	c.addRecent(v1ID)
	c.addRecent(v2ID)

	// 清理过多的缓存条目
	c.cleanupIfNeeded()
}

// getDistCache 从缓存获取向量距离
func (c *vectorCache) getDistCache(v1ID, v2ID string) (float32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if entry, ok := c.distCache[v1ID]; ok {
		if dist, found := entry[v2ID]; found {
			// 更新使用记录
			c.addRecent(v1ID)
			c.addRecent(v2ID)
			return dist, true
		}
	}
	return 0, false
}

// addQueryCache 添加查询结果到缓存
func (c *vectorCache) addQueryCache(key string, results []SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 创建副本以避免外部修改影响缓存
	cachedResults := make([]SearchResult, len(results))
	copy(cachedResults, results)

	c.queryResults[key] = cachedResults
	c.resultsAge[key] = time.Now()

	// 添加到最近使用列表
	c.mostRecent = append(c.mostRecent, key)
	if len(c.mostRecent) > c.maxCacheEntries {
		c.mostRecent = c.mostRecent[1:]
	}

	// 清理过时和过多的缓存
	c.cleanupIfNeeded()
}

// getQueryCache 从缓存获取查询结果
func (c *vectorCache) getQueryCache(key string) ([]SearchResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results, ok := c.queryResults[key]
	if !ok {
		return nil, false
	}

	// 检查结果是否过期
	age, ok := c.resultsAge[key]
	if !ok || time.Since(age) > c.maxResultsAge {
		return nil, false
	}

	// 创建副本以避免外部修改影响缓存
	cachedResults := make([]SearchResult, len(results))
	copy(cachedResults, results)

	return cachedResults, true
}

// addRecent 将ID添加到最近使用列表
func (c *vectorCache) addRecent(id string) {
	// 注：此方法假设调用者已经持有锁

	// 检查是否已经在列表中
	for i, existingID := range c.mostRecent {
		if existingID == id {
			// 如果已存在，移除旧位置
			c.mostRecent = append(c.mostRecent[:i], c.mostRecent[i+1:]...)
			break
		}
	}

	// 添加到最前面
	c.mostRecent = append([]string{id}, c.mostRecent...)

	// 保持列表长度在限制内
	if len(c.mostRecent) > c.maxCacheEntries {
		c.mostRecent = c.mostRecent[:c.maxCacheEntries]
	}
}

// cleanupIfNeeded 清理缓存中过期或过多的条目
func (c *vectorCache) cleanupIfNeeded() {
	// 注：此方法假设调用者已经持有锁

	// 清理过期的查询结果
	now := time.Now()
	for key, age := range c.resultsAge {
		if now.Sub(age) > c.maxResultsAge {
			delete(c.queryResults, key)
			delete(c.resultsAge, key)
		}
	}

	// 如果距离缓存太大，清理最不常用的条目
	if len(c.distCache) > c.maxCacheEntries*2 {
		// 构建使用频率映射
		usage := make(map[string]int)
		for _, id := range c.mostRecent {
			usage[id]++
		}

		// 找出使用最少的条目
		var toRemove []string
		for id := range c.distCache {
			if _, ok := usage[id]; !ok {
				toRemove = append(toRemove, id)
			}
		}

		// 删除最不常用的条目，最多删除总数的1/3
		maxToRemove := len(c.distCache) / 3
		if len(toRemove) > maxToRemove {
			toRemove = toRemove[:maxToRemove]
		}

		for _, id := range toRemove {
			delete(c.distCache, id)
		}
	}
}

// NewMemoryRepository 创建内存向量仓库
func NewMemoryRepository(config Config) (Repository, error) {
	// 确保维度大于0
	if config.Dimension <= 0 {
		return nil, fmt.Errorf("vector dimension must be positive")
	}

	// 确保距离类型有效
	distType := config.DistanceType
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
		vectorCache:    newVectorCache(),
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

	// 对于余弦距离，先对向量进行归一化处理
	if r.distType == Cosine {
		doc.Vector = normalizeVector(doc.Vector)
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

		// 对于余弦距离，对向量进行归一化处理
		if r.distType == Cosine {
			doc.Vector = normalizeVector(doc.Vector)
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
// 优化：使用缓存、并行处理和提前退出策略
func (r *MemoryRepository) Search(vector []float32, filter SearchFilter) ([]SearchResult, error) {
	// 验证向量
	if err := ValidateVector(vector, r.dimension); err != nil {
		return nil, err
	}

	// 对于余弦距离，对查询向量进行归一化处理
	if r.distType == Cosine {
		vector = normalizeVector(vector)
	}

	// 尝试从缓存获取查询结果
	cacheKey := cacheKey(vector, filter)
	if cachedResults, found := r.vectorCache.getQueryCache(cacheKey); found {
		return cachedResults, nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 过滤文档
	var filteredDocs []Document

	// 优化：使用索引直接查找FileID相关文档，而不是遍历所有文档
	if len(filter.FileIDs) > 0 {
		// 如果指定了文件ID，只检索这些文件的文档
		for _, fileID := range filter.FileIDs {
			docIDs, exists := r.fileToDocIDs[fileID]
			if !exists {
				continue
			}

			for _, docID := range docIDs {
				doc, exists := r.documents[docID]
				if exists && matchMetadata(doc.Metadata, filter.Metadata) {
					filteredDocs = append(filteredDocs, doc)
				}
			}
		}
	} else {
		// 否则检索所有文档并应用元数据过滤
		filteredDocs = make([]Document, 0, len(r.documents))
		for _, doc := range r.documents {
			if matchMetadata(doc.Metadata, filter.Metadata) {
				filteredDocs = append(filteredDocs, doc)
			}
		}
	}

	// 如果没有符合条件的文档，返回空结果
	if len(filteredDocs) == 0 {
		return []SearchResult{}, nil
	}

	// 优化：使用并发计算加速距离计算
	// 根据CPU核心数量决定线程数，但不超过可用核心数的80%
	threads := runtime.NumCPU() * 4 / 5
	if threads < 1 {
		threads = 1
	}
	// 对于小量文档不使用并发
	if len(filteredDocs) < 100 || threads == 1 {
		return r.serialSearch(vector, filteredDocs, filter)
	} else {
		return r.parallelSearch(vector, filteredDocs, filter, threads)
	}
}

// serialSearch 串行搜索实现
func (r *MemoryRepository) serialSearch(vector []float32, docs []Document, filter SearchFilter) ([]SearchResult, error) {
	results := make([]SearchResult, 0, len(docs))

	// 计算所有向量距离
	for _, doc := range docs {
		// 检查是否有缓存的距离计算结果
		queryKey := fmt.Sprintf("q_%f_%f", vector[0], vector[1])
		docKey := fmt.Sprintf("d_%s", doc.ID)

		var dist float32
		var err error

		// 尝试从缓存获取
		if cachedDist, found := r.vectorCache.getDistCache(queryKey, docKey); found {
			dist = cachedDist
		} else {
			// 计算距离
			dist, err = ComputeDistance(vector, doc.Vector, r.distType)
			if err != nil {
				return nil, fmt.Errorf("error computing distance: %v", err)
			}

			// 缓存计算结果
			r.vectorCache.addDistCache(queryKey, docKey, dist)
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
	SortSearchResults(results)

	// 只返回前N个结果
	if filter.MaxResults > 0 && len(results) > filter.MaxResults {
		results = results[:filter.MaxResults]
	}

	// 缓存查询结果
	cacheKey := cacheKey(vector, filter)
	r.vectorCache.addQueryCache(cacheKey, results)

	return results, nil
}

// parallelSearch 并行搜索实现
func (r *MemoryRepository) parallelSearch(vector []float32, docs []Document, filter SearchFilter, threads int) ([]SearchResult, error) {
	// 计算每个线程处理的文档数量
	docsPerThread := (len(docs) + threads - 1) / threads

	// 使用通道收集结果
	resultsChan := make(chan []SearchResult, threads)
	errorsChan := make(chan error, threads)

	// 启动多个goroutine进行并行计算
	for i := 0; i < threads; i++ {
		start := i * docsPerThread
		end := start + docsPerThread
		if end > len(docs) {
			end = len(docs)
		}

		if start >= end {
			continue
		}

		go func(start, end int) {
			threadResults := make([]SearchResult, 0, end-start)

			for j := start; j < end; j++ {
				doc := docs[j]

				// 与串行版本相同的距离计算逻辑
				queryKey := fmt.Sprintf("q_%f_%f", vector[0], vector[1])
				docKey := fmt.Sprintf("d_%s", doc.ID)

				var dist float32
				var err error

				// 尝试从缓存获取，注意并发安全
				if cachedDist, found := r.vectorCache.getDistCache(queryKey, docKey); found {
					dist = cachedDist
				} else {
					dist, err = ComputeDistance(vector, doc.Vector, r.distType)
					if err != nil {
						errorsChan <- fmt.Errorf("error computing distance: %v", err)
						return
					}
					r.vectorCache.addDistCache(queryKey, docKey, dist)
				}

				score := DistanceToScore(dist, r.distType)

				if score >= filter.MinScore {
					threadResults = append(threadResults, SearchResult{
						Document: doc,
						Score:    score,
						Distance: dist,
					})
				}
			}

			resultsChan <- threadResults
			errorsChan <- nil
		}(start, end)
	}

	// 收集结果和错误
	var allResults []SearchResult
	for i := 0; i < threads; i++ {
		if err := <-errorsChan; err != nil {
			return nil, err
		}
		allResults = append(allResults, <-resultsChan...)
	}

	// 排序并截取前N个结果
	SortSearchResults(allResults)

	if filter.MaxResults > 0 && len(allResults) > filter.MaxResults {
		allResults = allResults[:filter.MaxResults]
	}

	// 缓存查询结果
	cacheKey := cacheKey(vector, filter)
	r.vectorCache.addQueryCache(cacheKey, allResults)

	return allResults, nil
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

// GetDimension 返回向量维数
func (r *MemoryRepository) GetDimension() int {
	return r.dimension
}

// 在包初始化时注册内存仓库
func init() {
	RegisterRepository("memory", NewMemoryRepository)
}
