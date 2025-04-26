package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LocalStorage 本地文件存储实现
type LocalStorage struct {
	basePath string // 基础存储路径
}

// LocalConfig 本地存储配置
type LocalConfig struct {
	Path string // 本地存储路径
}

// NewLocalStorage 创建本地存储实例
func NewLocalStorage(cfg LocalConfig) (*LocalStorage, error) {
	// 确保路径是绝对路径
	absPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %v", err)
	}

	// 确保目录存在
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %v", err)
	}

	return &LocalStorage{
		basePath: absPath,
	}, nil
}

// Save 保存文件到本地存储
func (s *LocalStorage) Save(reader io.Reader, filename string) (FileInfo, error) {
	// 生成唯一标识符
	id := uuid.New().String()

	// 获取文件扩展名
	ext := filepath.Ext(filename)

	// 创建年月日目录结构，更好地组织文件
	now := time.Now()
	datePath := filepath.Join(fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()))

	// 完整的保存路径
	dirPath := filepath.Join(s.basePath, datePath)
	filePath := filepath.Join(dirPath, id+ext)

	// 创建目录
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return FileInfo{}, fmt.Errorf("failed to create directory: %v", err)
	}

	// 创建文件
	file, err := os.Create(filePath)
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// 写入文件内容
	size, err := io.Copy(file, reader)
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to write file: %v", err)
	}

	// 构建相对路径 (用于存储)
	relPath := filepath.Join(datePath, id+ext)

	// 返回文件信息
	return FileInfo{
		ID:       id,
		Name:     filename,
		Size:     size,
		MimeType: getMimeType(filename),
		Path:     relPath,
	}, nil
}

// Get 获取文件内容
func (s *LocalStorage) Get(id string) (io.ReadCloser, error) {
	// 查找文件
	filePath, err := s.findFilePathById(id)
	if err != nil {
		return nil, err
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	return file, nil
}

// Delete 删除文件
func (s *LocalStorage) Delete(id string) error {
	// 查找文件
	filePath, err := s.findFilePathById(id)
	if err != nil {
		return err
	}

	// 删除文件
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete file: %v", err)
	}

	return nil
}

// List 列出所有文件
func (s *LocalStorage) List() ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 获取相对路径
		relPath, err := filepath.Rel(s.basePath, path)
		if err != nil {
			return err
		}

		// 从文件名中提取ID
		fileName := filepath.Base(path)
		id := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		files = append(files, FileInfo{
			ID:       id,
			Name:     fileName,
			Size:     info.Size(),
			MimeType: getMimeType(fileName),
			Path:     relPath,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %v", err)
	}

	return files, nil
}

// Exists 检查文件是否存在
func (s *LocalStorage) Exists(id string) (bool, error) {
	_, err := s.findFilePathById(id)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// findFilePathById 根据ID查找文件路径
func (s *LocalStorage) findFilePathById(id string) (string, error) {
	var filePath string
	var found bool

	// 遍历查找匹配ID的文件
	err := filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			fileName := filepath.Base(path)
			fileId := strings.TrimSuffix(fileName, filepath.Ext(fileName))

			if fileId == id {
				filePath = path
				found = true
				return io.EOF // 用特殊错误来中断遍历
			}
		}

		return nil
	})

	// io.EOF 是我们用来中断遍历的信号，不是真正的错误
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("error searching for file: %v", err)
	}

	if !found {
		return "", fmt.Errorf("file with id %s not found", id)
	}

	return filePath, nil
}

// getMimeType 简单根据文件扩展名判断MIME类型
func getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".doc":
		return "application/msword"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}
