package vectordb

import (
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
