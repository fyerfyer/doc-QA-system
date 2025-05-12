package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DocumentStatus 文档处理状态类型
type DocumentStatus string

const (
	// DocStatusUploaded 文档已上传，等待处理
	DocStatusUploaded DocumentStatus = "uploaded"
	// DocStatusProcessing 文档处理中
	DocStatusProcessing DocumentStatus = "processing"
	// DocStatusCompleted 文档处理完成
	DocStatusCompleted DocumentStatus = "completed"
	// DocStatusFailed 文档处理失败
	DocStatusFailed DocumentStatus = "failed"
)

// ProcessStage 文档处理阶段
type ProcessStage string

const (
	// StageParsing 解析阶段
	StageParsing ProcessStage = "parsing"
	// StageChunking 分块阶段
	StageChunking ProcessStage = "chunking"
	// StageVectorizing 向量化阶段
	StageVectorizing ProcessStage = "vectorizing"
	// StageCompleted 处理完成
	StageCompleted ProcessStage = "completed"
)

// Document 文档数据模型
// 用于存储文档的元数据信息
type Document struct {
	ID             string         `gorm:"primaryKey"`         // 文档ID，主键
	FileName       string         `gorm:"not null"`           // 文件名
	FileType       string         `gorm:"not null"`           // 文件类型
	FilePath       string         `gorm:"not null"`           // 文件路径
	FileSize       int64          `gorm:"not null"`           // 文件大小（字节）
	Status         DocumentStatus `gorm:"not null;index"`     // 处理状态
	UploadedAt     time.Time      `gorm:"not null;index"`     // 上传时间
	ProcessedAt    *time.Time     `gorm:"index"`              // 处理完成时间
	UpdatedAt      time.Time      `gorm:"not null;index"`     // 更新时间
	Progress       int            `gorm:"not null;default:0"` // 处理进度（0-100）
	Error          string         `gorm:"type:text"`          // 错误信息
	SegmentCount   int            `gorm:"not null;default:0"` // 文档分段数量
	Tags           string         `gorm:"type:varchar(255)"`  // 标签，逗号分隔
	Metadata       datatypes.JSON `gorm:"type:json"`          // 元数据，JSON格式
	CurrentStage   ProcessStage   `gorm:"size:20"`            // 当前处理阶段
	CurrentTaskID  string         `gorm:"size:50;index"`      // 当前关联的任务ID
	PythonService  string         `gorm:"size:50"`            // 处理的Python服务名称
	LastTaskStatus string         `gorm:"size:20"`            // 最后任务的状态
	RetryCount     int            `gorm:"default:0"`          // 重试次数
}

// BeforeCreate GORM的钩子函数，创建记录前自动设置时间
func (d *Document) BeforeCreate(tx *gorm.DB) (err error) {
	// 如果上传时间为零值，设置为当前时间
	if d.UploadedAt.IsZero() {
		d.UploadedAt = time.Now()
	}
	// 设置更新时间
	d.UpdatedAt = time.Now()
	return nil
}

// BeforeUpdate GORM的钩子函数，更新记录前自动设置更新时间
func (d *Document) BeforeUpdate(tx *gorm.DB) (err error) {
	d.UpdatedAt = time.Now()
	return nil
}

// TableName 明确指定表名
func (Document) TableName() string {
	return "documents"
}

// DocumentSegment 文档分段数据模型
// 用于在数据库中跟踪文档的文本段落
type DocumentSegment struct {
	ID         uint           `gorm:"primaryKey;autoIncrement"` // 主键ID
	DocumentID string         `gorm:"not null;index"`           // 所属文档ID
	SegmentID  string         `gorm:"not null;uniqueIndex"`     // 段落唯一ID
	Position   int            `gorm:"not null"`                 // 段落位置
	Text       string         `gorm:"type:text;not null"`       // 段落文本内容
	CreatedAt  time.Time      `gorm:"not null"`                 // 创建时间
	UpdatedAt  time.Time      `gorm:"not null"`                 // 更新时间
	Metadata   datatypes.JSON `gorm:"type:json"`                // 段落元数据
	TaskID     string         `gorm:"size:50;index"`            // 处理此段落的任务ID
	VectorID   string         `gorm:"size:50"`                  // 向量数据库中的ID
}

// BeforeCreate GORM的钩子函数，创建记录前自动设置时间
func (ds *DocumentSegment) BeforeCreate(tx *gorm.DB) (err error) {
	now := time.Now()
	ds.CreatedAt = now
	ds.UpdatedAt = now
	return nil
}

// BeforeUpdate GORM的钩子函数，更新记录前自动设置更新时间
func (ds *DocumentSegment) BeforeUpdate(tx *gorm.DB) (err error) {
	ds.UpdatedAt = time.Now()
	return nil
}

// TableName 明确指定表名
func (DocumentSegment) TableName() string {
	return "document_segments"
}

// DocumentTask 文档任务关联模型
// 用于跟踪文档处理任务
type DocumentTask struct {
	ID         uint           `gorm:"primaryKey;autoIncrement"` // 主键ID
	DocumentID string         `gorm:"not null;index"`           // 文档ID
	TaskID     string         `gorm:"not null;uniqueIndex"`     // 任务ID
	TaskType   string         `gorm:"not null;size:50"`         // 任务类型
	Status     string         `gorm:"not null;size:20"`         // 任务状态
	CreatedAt  time.Time      `gorm:"not null"`                 // 创建时间
	UpdatedAt  time.Time      `gorm:"not null"`                 // 更新时间
	StartedAt  *time.Time     `gorm:""`                         // 开始时间
	EndedAt    *time.Time     `gorm:""`                         // 结束时间
	Error      string         `gorm:"type:text"`                // 错误信息
	Result     datatypes.JSON `gorm:"type:json"`                // 任务结果
	Retries    int            `gorm:"default:0"`                // 重试次数
	Progress   int            `gorm:"default:0"`                // 进度（0-100）
}

// BeforeCreate GORM的钩子函数，创建记录前自动设置时间
func (dt *DocumentTask) BeforeCreate(tx *gorm.DB) (err error) {
	now := time.Now()
	dt.CreatedAt = now
	dt.UpdatedAt = now
	return nil
}

// BeforeUpdate GORM的钩子函数，更新记录前自动设置更新时间
func (dt *DocumentTask) BeforeUpdate(tx *gorm.DB) (err error) {
	dt.UpdatedAt = time.Now()
	return nil
}

// TableName 明确指定表名
func (DocumentTask) TableName() string {
	return "document_tasks"
}
