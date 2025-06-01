package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioStorage MinIO存储实现
type MinioStorage struct {
	client     *minio.Client // MinIO客户端
	bucketName string        // 存储桶名称
}

// MinioConfig MinIO存储配置
type MinioConfig struct {
	Endpoint  string // MinIO服务端点
	AccessKey string // 访问密钥ID
	SecretKey string // 秘密访问密钥
	UseSSL    bool   // 是否使用SSL
	Bucket    string // 存储桶名称
}

// NewMinioStorage 创建MinIO存储实例
func NewMinioStorage(cfg MinioConfig) (*MinioStorage, error) {
	// 创建MinIO客户端
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}

	// 检查存储桶是否存在，不存在则创建
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check if bucket exists: %v", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %v", err)
		}
	}

	return &MinioStorage{
		client:     client,
		bucketName: cfg.Bucket,
	}, nil
}

// Save 保存文件到MinIO存储
func (s *MinioStorage) Save(reader io.Reader, filename string) (FileInfo, error) {
	// 生成唯一ID
	id := uuid.New().String()

	// 获取文件扩展名
	ext := filepath.Ext(filename)

	// 创建年月日目录结构
	now := time.Now()
	datePath := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())

	// 构建对象名
	objectName := fmt.Sprintf("%s/%s%s", datePath, id, ext)

	// 读取文件内容到内存，以获取大小和进行上传
	// 注意：对于大文件，应该使用流式上传而不是加载到内存
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to read file content: %v", err)
	}

	// 上传文件到MinIO
	contentReader := bytes.NewReader(content)
	size := int64(len(content))
	contentType := getMimeType(filename)

	_, err = s.client.PutObject(
		context.Background(),
		s.bucketName,
		objectName,
		contentReader,
		size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return FileInfo{}, fmt.Errorf("failed to upload file: %v", err)
	}

	// 返回文件信息
	return FileInfo{
		ID:       id,
		Name:     filename,
		Size:     size,
		MimeType: contentType,
		Path:     objectName,
	}, nil
}

// Get 获取MinIO中的文件
func (s *MinioStorage) Get(id string) (io.ReadCloser, error) {
	// 使用List操作查找文件
	files, err := s.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %v", err)
	}

	// 查找匹配ID的文件
	var objectName string
	for _, file := range files {
		if file.ID == id {
			objectName = file.Path
			break
		}
	}

	if objectName == "" {
		return nil, fmt.Errorf("file with id %s not found", id)
	}

	// 获取对象
	obj, err := s.client.GetObject(
		context.Background(),
		s.bucketName,
		objectName,
		minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %v", err)
	}

	return obj, nil
}

// Delete 从MinIO中删除文件
func (s *MinioStorage) Delete(id string) error {
	// 使用List操作查找文件
	files, err := s.List()
	if err != nil {
		return fmt.Errorf("failed to list files: %v", err)
	}

	// 查找匹配ID的文件
	var objectName string
	for _, file := range files {
		if file.ID == id {
			objectName = file.Path
			break
		}
	}

	if objectName == "" {
		return fmt.Errorf("file with id %s not found", id)
	}

	// 删除对象
	err = s.client.RemoveObject(
		context.Background(),
		s.bucketName,
		objectName,
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to delete object: %v", err)
	}

	return nil
}

// List 列出MinIO中的所有文件
func (s *MinioStorage) List() ([]FileInfo, error) {
	var files []FileInfo

	// 创建一个通道接收MinIO对象
	objectCh := s.client.ListObjects(
		context.Background(),
		s.bucketName,
		minio.ListObjectsOptions{Recursive: true},
	)

	// 遍历所有对象
	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("error listing objects: %v", object.Err)
		}

		// 从对象名称中提取ID
		objectName := object.Key
		fileName := filepath.Base(objectName)
		id := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		// 添加到文件列表
		files = append(files, FileInfo{
			ID:       id,
			Name:     fileName,
			Size:     object.Size,
			MimeType: getMimeTypeFromPath(objectName),
			Path:     objectName,
		})
	}

	return files, nil
}

// Exists 检查MinIO中是否存在指定ID的文件
func (s *MinioStorage) Exists(id string) (bool, error) {
	// 使用List操作查找文件
	files, err := s.List()
	if err != nil {
		return false, fmt.Errorf("failed to list files: %v", err)
	}

	// 查找匹配ID的文件
	for _, file := range files {
		if file.ID == id {
			return true, nil
		}
	}

	return false, nil
}

// getMimeTypeFromPath 从路径获取MIME类型
func getMimeTypeFromPath(path string) string {
	return getMimeType(path)
}
