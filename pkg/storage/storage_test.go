package storage

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// 创建测试文件辅助函数
func createTestFile(content string) (io.Reader, string) {
	return bytes.NewBufferString(content), fmt.Sprintf("test-%d.txt", os.Getpid())
}

// 读取文件内容辅助函数
func readAll(r io.Reader) string {
	b, _ := ioutil.ReadAll(r)
	return string(b)
}

// TestLocalStorage 测试本地存储实现
func TestLocalStorage(t *testing.T) {
	// 创建临时目录用于测试
	tempDir, err := ioutil.TempDir("", "docqa-test-*")
	if err != nil {
		t.Fatalf("Failed to create temporary test directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // 测试结束后清理

	// 初始化本地存储
	cfg := LocalConfig{
		Path: tempDir,
	}
	localStorage, err := NewLocalStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create local storage instance: %v", err)
	}

	// 测试 Save 功能
	t.Run("Save", func(t *testing.T) {
		content := "这是测试文件内容"
		fileReader, fileName := createTestFile(content)

		info, err := localStorage.Save(fileReader, fileName)
		if err != nil {
			t.Fatalf("Failed to save file: %v", err)
		}

		if info.ID == "" {
			t.Error("Returned file ID should not be empty")
		}

		if info.Name != fileName {
			t.Errorf("File name should be %s, got %s", fileName, info.Name)
		}

		// 检查文件是否确实被保存
		filePath := filepath.Join(tempDir, info.Path)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("File was not saved to disk: %s", filePath)
		}
	})

	// 保存一个文件用于后续测试
	content := "这是一个用于测试的样本文件"
	reader, fileName := createTestFile(content)
	fileInfo, err := localStorage.Save(reader, fileName)
	if err != nil {
		t.Fatalf("Failed to save test file: %v", err)
	}

	// 测试 Get 功能
	t.Run("Get", func(t *testing.T) {
		reader, err := localStorage.Get(fileInfo.ID)
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		defer reader.Close()

		retrievedContent := readAll(reader)
		if retrievedContent != content {
			t.Errorf("File content mismatch, expected: %s, got: %s", content, retrievedContent)
		}
	})

	// 测试 List 功能
	t.Run("List", func(t *testing.T) {
		files, err := localStorage.List()
		if err != nil {
			t.Fatalf("Failed to list files: %v", err)
		}

		if len(files) < 1 {
			t.Error("There should be at least one file, but the list is empty")
		}

		found := false
		for _, file := range files {
			if file.ID == fileInfo.ID {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Saved file ID not found: %s", fileInfo.ID)
		}
	})

	// 测试 Exists 功能
	t.Run("Exists", func(t *testing.T) {
		exists, err := localStorage.Exists(fileInfo.ID)
		if err != nil {
			t.Fatalf("Failed to check file existence: %v", err)
		}

		if !exists {
			t.Error("File should exist, but does not")
		}

		exists, err = localStorage.Exists("non-existent-id")
		if err != nil {
			t.Fatalf("Failed to check non-existent file: %v", err)
		}

		if exists {
			t.Error("Non-existent file should return false, but got true")
		}
	})

	// 测试 Delete 功能
	t.Run("Delete", func(t *testing.T) {
		err := localStorage.Delete(fileInfo.ID)
		if err != nil {
			t.Fatalf("Failed to delete file: %v", err)
		}

		// 确认文件已被删除
		exists, _ := localStorage.Exists(fileInfo.ID)
		if exists {
			t.Error("File should have been deleted, but still exists")
		}
	})
}

// TestMinioStorage 测试MinIO存储实现
// 需要运行docker-compose -f docker-compose.test.yml up -d先启动MinIO服务
func TestMinioStorage(t *testing.T) {
	// 如果环境变量SKIP_MINIO_TEST设置为true，则跳过MinIO测试
	if os.Getenv("SKIP_MINIO_TEST") == "true" {
		t.Skip("SKIP_MINIO_TEST environment variable set, skipping MinIO tests")
	}

	// MinIO测试配置
	cfg := MinioConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
		Bucket:    "docqa-test",
	}

	// 初始化MinIO存储
	minioStorage, err := NewMinioStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create MinIO storage: %v", err)
	}

	// 测试 Save 功能
	t.Run("Save", func(t *testing.T) {
		content := "这是MinIO测试文件内容"
		fileReader, fileName := createTestFile(content)

		info, err := minioStorage.Save(fileReader, fileName)
		if err != nil {
			t.Fatalf("Failed to save file to MinIO: %v", err)
		}

		if info.ID == "" {
			t.Error("Returned file ID should not be empty")
		}

		if info.Name != fileName {
			t.Errorf("File name should be %s, got %s", fileName, info.Name)
		}
	})

	// 保存一个文件用于后续测试
	content := "这是一个用于MinIO测试的样本文件"
	reader, fileName := createTestFile(content)
	fileInfo, err := minioStorage.Save(reader, fileName)
	if err != nil {
		t.Fatalf("Failed to save test file to MinIO: %v", err)
	}

	// 测试 Get 功能
	t.Run("Get", func(t *testing.T) {
		reader, err := minioStorage.Get(fileInfo.ID)
		if err != nil {
			t.Fatalf("Failed to get file from MinIO: %v", err)
		}
		defer reader.Close()

		retrievedContent := readAll(reader)
		if retrievedContent != content {
			t.Errorf("File content mismatch, expected: %s, got: %s", content, retrievedContent)
		}
	})

	// 测试 List 功能
	t.Run("List", func(t *testing.T) {
		files, err := minioStorage.List()
		if err != nil {
			t.Fatalf("Failed to list MinIO files: %v", err)
		}

		if len(files) < 1 {
			t.Error("There should be at least one file, but the list is empty")
		}

		found := false
		for _, file := range files {
			if file.ID == fileInfo.ID {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Saved file ID not found: %s", fileInfo.ID)
		}
	})

	// 测试 Exists 功能
	t.Run("Exists", func(t *testing.T) {
		exists, err := minioStorage.Exists(fileInfo.ID)
		if err != nil {
			t.Fatalf("Failed to check MinIO file existence: %v", err)
		}

		if !exists {
			t.Error("File should exist, but does not")
		}

		exists, err = minioStorage.Exists("non-existent-id")
		if err != nil {
			t.Fatalf("Failed to check non-existent file: %v", err)
		}

		if exists {
			t.Error("Non-existent file should return false, but got true")
		}
	})

	// 测试 Delete 功能
	t.Run("Delete", func(t *testing.T) {
		err := minioStorage.Delete(fileInfo.ID)
		if err != nil {
			t.Fatalf("Failed to delete MinIO file: %v", err)
		}

		// 确认文件已被删除
		exists, _ := minioStorage.Exists(fileInfo.ID)
		if exists {
			t.Error("File should have been deleted, but still exists")
		}
	})

	// 测试完成后清理测试桶
	cleanupTestBucket(t, minioStorage)
}

// cleanupTestBucket 清理测试桶中的所有对象
func cleanupTestBucket(t *testing.T, storage *MinioStorage) {
	t.Log("Cleaning up test bucket...")
	files, err := storage.List()
	if err != nil {
		t.Logf("Error listing objects for cleanup: %v", err)
		return
	}

	for _, file := range files {
		if err := storage.Delete(file.ID); err != nil {
			t.Logf("Failed to clean up object %s: %v", file.ID, err)
		}
	}
}

// TestStorageFactory 测试存储工厂函数
func TestStorageFactory(t *testing.T) {
	t.Run("NewLocalStorage", func(t *testing.T) {
		// 创建临时目录用于测试
		tempDir, err := ioutil.TempDir("", "docqa-factory-test-*")
		if err != nil {
			t.Fatalf("Failed to create temporary test directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		cfg := LocalConfig{
			Path: tempDir,
		}

		storage, err := NewLocalStorage(cfg)
		if err != nil {
			t.Fatalf("Failed to create local storage: %v", err)
		}

		if storage == nil {
			t.Fatal("Created storage instance should not be nil")
		}

		// 验证存储路径已创建
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			t.Errorf("Storage path was not created: %s", tempDir)
		}
	})
}
