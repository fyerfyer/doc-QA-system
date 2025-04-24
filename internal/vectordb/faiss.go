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
	mu             sync.RWMutex
	index          faiss.Index // 改为接口类型
	documents      map[string]Document
	fileToDocIDs   map[string][]string
	idToPosition   map[string]int
	indexPath      string
	metaPath       string
	dimension      int
	distanceType   DistanceType
	saveOnClose    bool
	autoSave       bool
	autoSaveCount  int
	operationCount int
}

// NewFaissRepository 创建新的Faiss向量仓库
func NewFaissRepository(config Config) (Repository, error) {
	if config.Dimension <= 0 {
		return nil, fmt.Errorf("vector dimension must be positive")
	}

	if config.Path != "" && !config.InMemory {
		if err := os.MkdirAll(filepath.Dir(config.Path), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %v", err)
		}
	}

	distType := config.DistanceType
	if distType == "" {
		distType = Cosine
	}

	base := NewBaseRepository(config.Dimension, distType)
	indexPath := config.Path
	metaPath := ""
	if indexPath != "" {
		metaPath = indexPath + ".meta.json"
	}

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
		autoSaveCount:  100,
	}

	var index faiss.Index
	var err error

	// 尝试从文件加载索引
	if indexPath != "" && !config.InMemory && fileExists(indexPath) {
		index, err = faiss.ReadIndex(indexPath, 0)
		if err != nil {
			if config.CreateIfNotExists {
				index, err = createFaissIndex(config.Dimension, distType)
				if err != nil {
					return nil, fmt.Errorf("failed to create Faiss index: %v", err)
				}
			} else {
				return nil, fmt.Errorf("failed to read index file: %v", err)
			}
		} else {
			if err := repo.loadMetadata(metaPath); err != nil {
				return nil, fmt.Errorf("failed to load documents metadata: %v", err)
			}
		}
	} else {
		index, err = createFaissIndex(config.Dimension, distType)
		if err != nil {
			return nil, fmt.Errorf("failed to create Faiss index: %v", err)
		}
	}

	repo.index = index
	return repo, nil
}

// createFaissIndex 创建Faiss索引
func createFaissIndex(dimension int, distType DistanceType) (faiss.Index, error) {
	var metric int
	switch distType {
	case Cosine, DotProduct:
		metric = faiss.MetricInnerProduct
	case Euclidean:
		metric = faiss.MetricL2
	default:
		metric = faiss.MetricL2
	}
	return faiss.NewIndexFlat(dimension, metric)
}

// Add 添加单个文档到仓库
func (r *FaissRepository) Add(doc Document) error {
	if err := ValidateVector(doc.Vector, r.dimension); err != nil {
		return err
	}
	if r.distanceType == Cosine {
		doc.Vector = normalizeVector(doc.Vector)
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}
	if doc.Metadata == nil {
		doc.Metadata = make(map[string]interface{})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	nextPos := int(r.index.Ntotal())
	err := r.index.Add(doc.Vector)
	if err != nil {
		return fmt.Errorf("failed to add vector to index: %v", err)
	}

	r.documents[doc.ID] = doc
	r.idToPosition[doc.ID] = nextPos
	r.fileToDocIDs[doc.FileID] = append(r.fileToDocIDs[doc.FileID], doc.ID)
	r.operationCount++

	if r.autoSave && r.operationCount >= r.autoSaveCount {
		if err := r.saveIndex(); err != nil {
			return fmt.Errorf("auto-save failed: %v", err)
		}
		r.operationCount = 0
	}
	return nil
}

// AddBatch 批量添加文档到仓库
func (r *FaissRepository) AddBatch(docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	vectors := make([][]float32, len(docs))
	for i := range docs {
		if err := ValidateVector(docs[i].Vector, r.dimension); err != nil {
			return fmt.Errorf("invalid vector for document %s: %v", docs[i].ID, err)
		}
		if r.distanceType == Cosine {
			docs[i].Vector = normalizeVector(docs[i].Vector)
		}
		vectors[i] = docs[i].Vector
		if docs[i].CreatedAt.IsZero() {
			docs[i].CreatedAt = time.Now()
		}
		if docs[i].Metadata == nil {
			docs[i].Metadata = make(map[string]interface{})
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	startPos := int(r.index.Ntotal())
	for _, vector := range vectors {
		if err := ValidateVector(vector, r.dimension); err != nil {
			return fmt.Errorf("invalid vector: %v", err)
		}

		if err := r.index.Add(vector); err != nil {
			return fmt.Errorf("failed to add vector to index: %v", err)
		}
	}

	for i, doc := range docs {
		position := startPos + i
		r.documents[doc.ID] = doc
		r.idToPosition[doc.ID] = position
		r.fileToDocIDs[doc.FileID] = append(r.fileToDocIDs[doc.FileID], doc.ID)
	}
	r.operationCount += len(docs)
	if r.autoSave && r.operationCount >= r.autoSaveCount {
		if err := r.saveIndex(); err != nil {
			return fmt.Errorf("auto-save failed: %v", err)
		}
		r.operationCount = 0
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
	doc, exists := r.documents[id]
	if !exists {
		return ErrDocumentNotFound
	}
	delete(r.documents, id)
	delete(r.idToPosition, id)
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
	r.operationCount++
	return nil
}

// DeleteByFileID 删除指定文件的所有文档
func (r *FaissRepository) DeleteByFileID(fileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	docIDs, exists := r.fileToDocIDs[fileID]
	if !exists {
		return nil
	}
	for _, id := range docIDs {
		delete(r.documents, id)
		delete(r.idToPosition, id)
	}
	delete(r.fileToDocIDs, fileID)
	r.operationCount += len(docIDs)
	return nil
}

// Search 相似度搜索
func (r *FaissRepository) Search(vector []float32, filter SearchFilter) ([]SearchResult, error) {
	if err := ValidateVector(vector, r.dimension); err != nil {
		return nil, err
	}
	if r.distanceType == Cosine {
		vector = normalizeVector(vector)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.documents) == 0 {
		return []SearchResult{}, nil
	}
	k := filter.MaxResults
	if k <= 0 {
		k = 10
	}
	queryLimit := k * 2
	total := int(r.index.Ntotal())
	if queryLimit > total {
		queryLimit = total
	}
	if queryLimit == 0 {
		return []SearchResult{}, nil
	}
	distances, indices, err := r.index.Search(vector, int64(queryLimit))
	if err != nil {
		return nil, fmt.Errorf("failed to search index: %v", err)
	}
	var results []SearchResult
	for i := 0; i < len(indices); i++ {
		idx := indices[i]
		if idx < 0 {
			continue
		}
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
			continue
		}
		doc, exists := r.documents[docID]
		if !exists {
			continue
		}
		if len(filter.FileIDs) > 0 {
			found := false
			for _, id := range filter.FileIDs {
				if doc.FileID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if !matchMetadata(doc.Metadata, filter.Metadata) {
			continue
		}
		dist := distances[i]
		score := DistanceToScore(dist, r.distanceType)
		if score < filter.MinScore {
			continue
		}
		results = append(results, SearchResult{
			Document: doc,
			Score:    score,
			Distance: dist,
		})
		if len(results) >= k {
			break
		}
	}
	SortSearchResults(results)
	return results, nil
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
	if r.saveOnClose && r.indexPath != "" {
		if err := r.saveIndex(); err != nil {
			return fmt.Errorf("failed to save index on close: %v", err)
		}
	}
	return nil
}

// saveIndex 保存索引和文档数据到文件
func (r *FaissRepository) saveIndex() error {
	if r.indexPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.indexPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	if err := faiss.WriteIndex(r.index, r.indexPath); err != nil {
		return fmt.Errorf("failed to write index to file: %v", err)
	}
	return r.saveMetadata()
}

// saveMetadata 保存文档元数据到文件
func (r *FaissRepository) saveMetadata() error {
	if r.metaPath == "" {
		return nil
	}
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
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(r.metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %v", err)
	}
	return nil
}

// loadMetadata 从文件加载文档元数据
func (r *FaissRepository) loadMetadata(path string) error {
	if path == "" {
		return nil
	}
	if !fileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read metadata file: %v", err)
	}
	metadata := struct {
		Documents      map[string]Document `json:"documents"`
		FileToDocIDs   map[string][]string `json:"file_to_doc_ids"`
		IDToPosition   map[string]int      `json:"id_to_position"`
		OperationCount int                 `json:"operation_count"`
	}{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %v", err)
	}
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

func init() {
	RegisterRepository("faiss", NewFaissRepository)
}
