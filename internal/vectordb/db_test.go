package vectordb

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDoc 创建用于测试的文档
func createTestDoc(id, fileID string, position int, vector []float32) Document {
	return Document{
		ID:       id,
		FileID:   fileID,
		Position: position,
		Text:     "这是测试文档 " + id,
		Vector:   vector,
		Metadata: map[string]interface{}{
			"source": "test",
			"lang":   "zh",
		},
		CreatedAt: time.Now(),
	}
}

// TestMemoryRepository 测试内存向量仓库
func TestMemoryRepository(t *testing.T) {
	// 创建内存仓库
	config := Config{
		Type:         "memory",
		Dimension:    4,
		DistanceType: DistanceType(string(Cosine)),
	}

	repo, err := NewRepository(config)
	require.NoError(t, err)
	defer repo.Close()

	testRepository(t, repo)
}

// TestFaissRepository 测试FAISS向量仓库
func TestFaissRepository(t *testing.T) {
	// 创建临时目录用于测试
	tempDir := filepath.Join(os.TempDir(), "faiss_test")
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	indexPath := filepath.Join(tempDir, "test_index")

	// 创建FAISS仓库
	config := Config{
		Type:              "faiss",
		Dimension:         4,
		DistanceType:      DistanceType(string(Cosine)),
		Path:              indexPath,
		CreateIfNotExists: true,
	}

	repo, err := NewRepository(config)
	if err != nil {
		t.Skip("FAISS may not be installed correctly, skipping test: " + err.Error())
	}
	defer repo.Close()

	testRepository(t, repo)
}

// TestFaissSaveAndLoad 测试FAISS索引的保存和加载功能
func TestFaissSaveAndLoad(t *testing.T) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), "faiss_save_load_test")
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	indexPath := filepath.Join(tempDir, "save_load_index")

	// 创建向量
	v1 := []float32{0.1, 0.2, 0.3, 0.4}
	v2 := []float32{0.5, 0.6, 0.7, 0.8}

	// 第一步：创建并填充索引
	{
		config := Config{
			Type:              "faiss",
			Dimension:         4,
			DistanceType:      Cosine,
			Path:              indexPath,
			CreateIfNotExists: true,
		}

		repo, err := NewRepository(config)
		if err != nil {
			t.Skip("FAISS may not be installed correctly, skipping test: " + err.Error())
		}

		// 添加测试文档
		doc1 := createTestDoc("doc1", "file1", 1, v1)
		doc2 := createTestDoc("doc2", "file1", 2, v2)

		err = repo.Add(doc1)
		require.NoError(t, err)

		err = repo.Add(doc2)
		require.NoError(t, err)

		// 关闭仓库，这将触发索引保存
		err = repo.Close()
		require.NoError(t, err)
	}

	// 第二步：加载索引并验证数据
	{
		config := Config{
			Type:         "faiss",
			Dimension:    4,
			DistanceType: Cosine,
			Path:         indexPath,
		}

		repo, err := NewRepository(config)
		require.NoError(t, err)
		defer repo.Close()

		// 检查文档是否正确加载
		doc1, err := repo.Get("doc1")
		require.NoError(t, err)
		assert.Equal(t, "doc1", doc1.ID)

		doc2, err := repo.Get("doc2")
		require.NoError(t, err)
		assert.Equal(t, "doc2", doc2.ID)

		// 测试搜索功能是否正常工作
		searchVector := []float32{0.15, 0.25, 0.35, 0.45} // 接近v1的向量
		filter := DefaultSearchFilter()
		filter.MaxResults = 1

		results, err := repo.Search(searchVector, filter)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "doc1", results[0].Document.ID)
	}
}

// TestFaissAutoSave 测试FAISS的自动保存功能
func TestFaissAutoSave(t *testing.T) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), "faiss_autosave_test")
	err := os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	indexPath := filepath.Join(tempDir, "autosave_index")

	// 创建配置，设置较小的自动保存阈值
	config := Config{
		Type:              "faiss",
		Dimension:         4,
		DistanceType:      Cosine,
		Path:              indexPath,
		CreateIfNotExists: true,
	}

	repo, err := NewFaissRepository(config)
	if err != nil {
		t.Skip("FAISS may not be installed correctly, skipping test: " + err.Error())
	}

	// 强制设置较小的自动保存阈值，便于测试
	faissRepo, ok := repo.(*FaissRepository)
	require.True(t, ok)
	faissRepo.autoSaveCount = 3

	// 添加文档触发自动保存
	for i := 0; i < 5; i++ {
		doc := createTestDoc(
			fmt.Sprintf("autosave_doc_%d", i),
			"file1",
			i,
			[]float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3, float32(i) * 0.4},
		)
		err := repo.Add(doc)
		require.NoError(t, err)
	}

	// 关闭仓库
	err = repo.Close()
	require.NoError(t, err)

	// 确认索引文件和元数据文件存在
	_, err = os.Stat(indexPath)
	assert.NoError(t, err)
	_, err = os.Stat(indexPath + ".meta.json")
	assert.NoError(t, err)

	// 重新加载索引测试
	newRepo, err := NewRepository(config)
	require.NoError(t, err)
	defer newRepo.Close()

	// 检查是否成功加载所有文档
	count, err := newRepo.Count()
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

// TestFaissSearchWithFilters 测试FAISS的过滤搜索功能
func TestFaissSearchWithFilters(t *testing.T) {
	// 创建内存模式的FAISS仓库进行测试
	config := Config{
		Type:         "faiss",
		Dimension:    4,
		DistanceType: Cosine,
		InMemory:     true,
	}

	repo, err := NewRepository(config)
	if err != nil {
		t.Skip("FAISS may not be installed correctly, skipping test: " + err.Error())
	}
	defer repo.Close()

	// 创建测试文档集，包含不同fileID和metadata
	docs := []Document{
		createTestDoc("doc1", "file1", 1, []float32{0.1, 0.2, 0.3, 0.4}),
		createTestDoc("doc2", "file1", 2, []float32{0.5, 0.6, 0.7, 0.8}),
		createTestDoc("doc3", "file2", 1, []float32{0.9, 0.8, 0.7, 0.6}),
	}

	// 添加自定义元数据
	docs[0].Metadata["category"] = "tech"
	docs[1].Metadata["category"] = "tech"
	docs[2].Metadata["category"] = "science"

	// 批量添加文档
	err = repo.AddBatch(docs)
	require.NoError(t, err)

	// 测试按fileID过滤
	t.Run("filter by file ID", func(t *testing.T) {
		filter := DefaultSearchFilter()
		filter.FileIDs = []string{"file1"}

		searchVector := []float32{0.5, 0.5, 0.5, 0.5}
		results, err := repo.Search(searchVector, filter)
		require.NoError(t, err)

		// 应该只返回file1的文档
		assert.Len(t, results, 2)
		for _, res := range results {
			assert.Equal(t, "file1", res.Document.FileID)
		}
	})

	// 测试按元数据过滤
	t.Run("filter by metadata", func(t *testing.T) {
		filter := DefaultSearchFilter()
		filter.Metadata = map[string]interface{}{
			"category": "tech",
		}

		searchVector := []float32{0.5, 0.5, 0.5, 0.5}
		results, err := repo.Search(searchVector, filter)
		require.NoError(t, err)

		// 应该只返回category为tech的文档
		assert.Len(t, results, 2)
		for _, res := range results {
			assert.Equal(t, "tech", res.Document.Metadata["category"])
		}
	})

	// 测试组合过滤
	t.Run("combined filters", func(t *testing.T) {
		filter := DefaultSearchFilter()
		filter.FileIDs = []string{"file1"}
		filter.Metadata = map[string]interface{}{
			"category": "tech",
		}

		searchVector := []float32{0.5, 0.5, 0.5, 0.5}
		results, err := repo.Search(searchVector, filter)
		require.NoError(t, err)

		// 应该只返回file1且category为tech的文档
		assert.Len(t, results, 2)
		for _, res := range results {
			assert.Equal(t, "file1", res.Document.FileID)
			assert.Equal(t, "tech", res.Document.Metadata["category"])
		}
	})

	// 测试最小分数过滤
	t.Run("min score filter", func(t *testing.T) {
		filter := DefaultSearchFilter()
		filter.MinScore = 0.9 // 设置较高阈值

		searchVector := []float32{0.1, 0.2, 0.3, 0.4} // 与doc1非常相似
		results, err := repo.Search(searchVector, filter)
		require.NoError(t, err)

		// 应该只返回与查询向量高度相似的文档
		if len(results) > 0 {
			assert.Equal(t, "doc1", results[0].Document.ID)
		}
	})
}

// TestQueryCache 测试查询缓存功能
func TestQueryCache(t *testing.T) {
	config := Config{
		Type:         "faiss",
		Dimension:    4,
		DistanceType: Cosine,
		InMemory:     true,
	}

	repo, err := NewFaissRepository(config)
	if err != nil {
		t.Skip("FAISS may not be installed correctly, skipping test: " + err.Error())
	}
	defer repo.Close()

	// 添加测试文档
	docs := []Document{
		createTestDoc("doc1", "file1", 1, []float32{0.1, 0.2, 0.3, 0.4}),
		createTestDoc("doc2", "file1", 2, []float32{0.5, 0.6, 0.7, 0.8}),
	}

	err = repo.AddBatch(docs)
	require.NoError(t, err)

	// 第一次搜索，应该执行实际查询
	searchVector := []float32{0.1, 0.2, 0.3, 0.4}
	filter := DefaultSearchFilter()
	filter.MaxResults = 2

	results1, err := repo.Search(searchVector, filter)
	require.NoError(t, err)
	require.Len(t, results1, 2)

	// 修改一个文档的元数据，但保持ID不变
	faissRepo := repo.(*FaissRepository)
	doc := faissRepo.documents["doc1"]
	doc.Metadata["updated"] = true
	faissRepo.documents["doc1"] = doc

	// 第二次使用完全相同的参数搜索，应该返回缓存结果
	results2, err := repo.Search(searchVector, filter)
	require.NoError(t, err)
	require.Len(t, results2, 2)

	// 验证第二次搜索结果确实来自缓存(不反映元数据的修改)
	assert.Equal(t, results1[0].Document.ID, results2[0].Document.ID)
	_, hasUpdated := results2[0].Document.Metadata["updated"]
	assert.False(t, hasUpdated, "搜索结果应该来自缓存，不反映元数据更新")

	// 清除缓存(通过删除操作)
	err = repo.Delete("doc2")
	require.NoError(t, err)

	// 第三次搜索，应该重新执行查询
	results3, err := repo.Search(searchVector, filter)
	require.NoError(t, err)
	require.Len(t, results3, 1) // 只剩一个文档

	// 确认结果反映了元数据的更改
	_, hasUpdated = results3[0].Document.Metadata["updated"]
	assert.True(t, hasUpdated, "删除操作应该清除了缓存，第三次搜索结果应反映最新状态")
}

// testRepository 向量仓库通用测试逻辑
func testRepository(t *testing.T, repo Repository) {
	// 创建测试向量 - 使用不同的值以便搜索时有明确区分
	v1 := []float32{0.1, 0.2, 0.3, 0.4} // 较小的值
	v2 := []float32{0.5, 0.5, 0.5, 0.5} // 中等值
	v3 := []float32{0.7, 0.8, 0.9, 1.0} // 较大的值

	// 1. 测试添加单个文档
	t.Run("add single doc", func(t *testing.T) {
		doc1 := createTestDoc("doc1", "file1", 1, v1)
		err := repo.Add(doc1)
		require.NoError(t, err)

		// 验证文档已添加
		result, err := repo.Get("doc1")
		require.NoError(t, err)
		assert.Equal(t, doc1.ID, result.ID)
		assert.Equal(t, doc1.FileID, result.FileID)
	})

	// 2. 测试批量添加文档
	t.Run("batch insert docs", func(t *testing.T) {
		docs := []Document{
			createTestDoc("doc2", "file1", 2, v2),
			createTestDoc("doc3", "file2", 1, v3),
		}
		err := repo.AddBatch(docs)
		require.NoError(t, err)

		// 验证文档数量
		count, err := repo.Count()
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	// 3. 测试向量搜索
	t.Run("vector search", func(t *testing.T) {
		// 使用接近v2的向量进行搜索
		searchVector := []float32{0.45, 0.55, 0.45, 0.55}
		filter := DefaultSearchFilter()
		filter.MaxResults = 2

		results, err := repo.Search(searchVector, filter)
		require.NoError(t, err)
		require.NotEmpty(t, results)

		// 最相似的应该是doc2
		assert.Equal(t, "doc2", results[0].Document.ID)
	})

	// 4. 测试过滤搜索
	t.Run("filter search", func(t *testing.T) {
		searchVector := []float32{0.5, 0.5, 0.5, 0.5}

		// 按文件ID过滤
		fileFilter := DefaultSearchFilter()
		fileFilter.FileIDs = []string{"file2"}

		fileResults, err := repo.Search(searchVector, fileFilter)
		require.NoError(t, err)
		for _, res := range fileResults {
			assert.Equal(t, "file2", res.Document.FileID)
		}

		// 按元数据过滤
		metaFilter := DefaultSearchFilter()
		metaFilter.Metadata = map[string]interface{}{
			"lang": "zh",
		}

		metaResults, err := repo.Search(searchVector, metaFilter)
		require.NoError(t, err)
		assert.NotEmpty(t, metaResults)
	})

	// 5. 测试删除单个文档
	t.Run("delete single doc", func(t *testing.T) {
		err := repo.Delete("doc1")
		require.NoError(t, err)

		_, err = repo.Get("doc1")
		assert.Error(t, err)
	})

	// 6. 测试按文件ID删除
	t.Run("delete by file ID", func(t *testing.T) {
		err := repo.DeleteByFileID("file2")
		require.NoError(t, err)

		// 验证删除后只剩下doc2
		count, err := repo.Count()
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// 确认doc2还在
		doc, err := repo.Get("doc2")
		require.NoError(t, err)
		assert.Equal(t, "doc2", doc.ID)
	})
}

// TestTimedCacheBasics 测试TimedCache的基本功能
func TestTimedCacheBasics(t *testing.T) {
	// 创建一个较长TTL的缓存以便测试
	cache := NewTimedCache(5 * time.Second)
	assert.NotNil(t, cache, "Cache should be created")

	// 测试设置和获取
	cache.Set("key1", "value1")
	val, found := cache.Get("key1")
	assert.True(t, found, "Key should be found")
	assert.Equal(t, "value1", val, "Value should match")

	// 测试覆盖已存在的值
	cache.Set("key1", "updated_value")
	val, found = cache.Get("key1")
	assert.True(t, found, "Key should be found after update")
	assert.Equal(t, "updated_value", val, "Value should be updated")

	// 测试不存在的键
	val, found = cache.Get("non_existent")
	assert.False(t, found, "Non-existent key should not be found")
	assert.Nil(t, val, "Value for non-existent key should be nil")
}

// TestTimedCacheExpiration 测试缓存过期功能
func TestTimedCacheExpiration(t *testing.T) {
	// 创建一个短TTL的缓存
	shortTTL := 100 * time.Millisecond
	cache := NewTimedCache(shortTTL)

	// 设置测试值
	cache.Set("expires_soon", "temp_value")

	// 立即获取，应该能找到
	val, found := cache.Get("expires_soon")
	assert.True(t, found, "Key should be found before expiration")
	assert.Equal(t, "temp_value", val, "Value should match before expiration")

	// 等待超过TTL的时间
	time.Sleep(shortTTL * 2)

	// 再次获取，应该已经过期
	val, found = cache.Get("expires_soon")
	assert.False(t, found, "Key should not be found after expiration")
	assert.Nil(t, val, "Value should be nil after expiration")
}

// TestTimedCacheCleanup 测试缓存清理功能
func TestTimedCacheCleanup(t *testing.T) {
	// 创建一个短TTL的缓存
	shortTTL := 100 * time.Millisecond
	cache := NewTimedCache(shortTTL)

	// 添加多个测试值
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// 验证所有值都已添加
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")
	assert.True(t, found1 && found2 && found3, "All keys should be found initially")

	// 手动触发清理
	cache.Cleanup()

	// 验证清理前的键仍然存在（因为还未过期）
	_, found1 = cache.Get("key1")
	_, found2 = cache.Get("key2")
	_, found3 = cache.Get("key3")
	assert.True(t, found1 && found2 && found3, "Keys should still exist after cleanup if not expired")

	// 等待超过TTL的时间
	time.Sleep(shortTTL * 2)

	// 手动触发清理
	cache.Cleanup()

	// 验证键已被清理
	_, found1 = cache.Get("key1")
	_, found2 = cache.Get("key2")
	_, found3 = cache.Get("key3")
	assert.False(t, found1 || found2 || found3, "All keys should be removed after expiration and cleanup")
}

// TestTimedCacheMultipleValues 测试缓存中存储不同类型的值
func TestTimedCacheMultipleValues(t *testing.T) {
	cache := NewTimedCache(5 * time.Second)

	// 测试不同类型的值
	testCases := []struct {
		key   string
		value interface{}
	}{
		{"string_key", "string_value"},
		{"int_key", 42},
		{"float_key", 3.14},
		{"bool_key", true},
		{"slice_key", []string{"a", "b", "c"}},
		{"map_key", map[string]int{"one": 1, "two": 2}},
	}

	// 设置所有测试值
	for _, tc := range testCases {
		cache.Set(tc.key, tc.value)
	}

	// 验证所有值都正确存储
	for _, tc := range testCases {
		val, found := cache.Get(tc.key)
		assert.True(t, found, "Key %s should be found", tc.key)
		assert.Equal(t, tc.value, val, "Value for key %s should match", tc.key)
	}
}

// TestTimedCacheConcurrentAccess 测试并发访问场景
func TestTimedCacheConcurrentAccess(t *testing.T) {
	cache := NewTimedCache(5 * time.Second)
	const concurrentRoutines = 10
	const operationsPerRoutine = 100

	// 使用通道作为同步机制
	done := make(chan bool, concurrentRoutines)

	// 启动多个goroutine同时操作缓存
	for i := 0; i < concurrentRoutines; i++ {
		go func(routineID int) {
			baseKey := "key_" + string(rune('A'+routineID))

			// 执行多次设置和获取操作
			for j := 0; j < operationsPerRoutine; j++ {
				key := baseKey + string(rune('0'+j%10))
				value := routineID*1000 + j

				cache.Set(key, value)
				val, _ := cache.Get(key)

				// 检查值是否正确，但不使用assert以避免并发问题
				if val != value {
					t.Errorf("Concurrent value mismatch: expected %v, got %v", value, val)
				}
			}

			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < concurrentRoutines; i++ {
		<-done
	}
}
