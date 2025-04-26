package model

import (
	"mime/multipart"
	"time"
)

// 分页请求参数
type PaginationRequest struct {
	Page     int `form:"page" json:"page" binding:"omitempty,min=1"`           // 当前页码，从1开始
	PageSize int `form:"page_size" json:"page_size" binding:"omitempty,min=1"` // 每页记录数
}

// GetPage 获取页码，默认为1
func (p *PaginationRequest) GetPage() int {
	if p.Page <= 0 {
		return 1
	}
	return p.Page
}

// GetPageSize 获取每页记录数，默认为10，最大为100
func (p *PaginationRequest) GetPageSize() int {
	if p.PageSize <= 0 {
		return 10
	}
	if p.PageSize > 100 {
		return 100
	}
	return p.PageSize
}

// DocumentUploadRequest 文档上传请求
type DocumentUploadRequest struct {
	File     *multipart.FileHeader `form:"file" binding:"required"`                      // 文件对象
	Tags     string                `form:"tags" json:"tags" binding:"omitempty"`         // 文档标签，逗号分隔
	Metadata map[string]string     `form:"metadata" json:"metadata" binding:"omitempty"` // 文档元数据
}

// DocumentStatusRequest 文档状态查询请求
type DocumentStatusRequest struct {
	ID string `uri:"id" binding:"required"` // 文档ID
}

// DocumentListRequest 文档列表请求
type DocumentListRequest struct {
	PaginationRequest
	StartTime *time.Time `form:"start_time" json:"start_time" binding:"omitempty"` // 开始时间
	EndTime   *time.Time `form:"end_time" json:"end_time" binding:"omitempty"`     // 结束时间
	Status    string     `form:"status" json:"status" binding:"omitempty"`         // 文档状态
	Tags      string     `form:"tags" json:"tags" binding:"omitempty"`             // 标签过滤
}

// DocumentDeleteRequest 文档删除请求
type DocumentDeleteRequest struct {
	ID string `uri:"id" binding:"required"` // 文档ID
}

// QARequest 问答请求
type QARequest struct {
	Question  string                 `json:"question" binding:"required"`          // 问题内容
	FileID    string                 `json:"file_id" binding:"omitempty"`          // 可选的文件ID，指定从特定文件中回答
	Metadata  map[string]interface{} `json:"metadata" binding:"omitempty"`         // 可选的元数据过滤
	MaxTokens int                    `json:"max_tokens" binding:"omitempty,min=1"` // 可选的最大生成tokens数量
}
