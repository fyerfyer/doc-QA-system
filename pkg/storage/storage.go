package storage

import (
	"io"
)

// FileInfo 文件元数据结构
type FileInfo struct {
	ID       string // 文件唯一标识符
	Name     string // 原始文件名
	Size     int64  // 文件大小(字节)
	MimeType string // 文件MIME类型(可选)
	Path     string // 内部存储路径(实现相关)
}

// Storage 文件存储接口
// 定义文件存储的基本操作，可以有不同实现(本地文件系统、MinIO等)
type Storage interface {
	// Save 保存文件并返回文件信息
	Save(reader io.Reader, filename string) (FileInfo, error)

	// Get 获取文件内容
	Get(id string) (io.ReadCloser, error)

	// Delete 删除文件
	Delete(id string) error

	// List 列出所有文件
	List() ([]FileInfo, error)

	// Exists 检查文件是否存在
	Exists(id string) (bool, error)
}

// Factory 存储实现的工厂函数
// 用于根据配置创建不同类型的存储实现
type Factory func(cfg interface{}) (Storage, error)
