package model

import (
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
)

// Response 通用响应结构
type Response struct {
	Code    int         `json:"code"`               // 响应状态码，0表示成功
	Message string      `json:"message"`            // 响应消息
	Data    interface{} `json:"data,omitempty"`     // 响应数据，可能为空
	TraceID string      `json:"trace_id,omitempty"` // 调用链追踪ID
}

// NewSuccessResponse 创建成功响应
func NewSuccessResponse(data interface{}) *Response {
	return &Response{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(code int, message string) *Response {
	return &Response{
		Code:    code,
		Message: message,
	}
}

// DocumentUploadResponse 文档上传响应
type DocumentUploadResponse struct {
	FileID   string `json:"file_id"`  // 文件ID
	FileName string `json:"filename"` // 文件名
	Status   string `json:"status"`   // 文档状态：pending、processing、completed、failed
}

// DocumentStatusResponse 文档状态查询响应
type DocumentStatusResponse struct {
	FileID    string `json:"file_id"`            // 文档ID
	Status    string `json:"status"`             // 处理状态
	FileName  string `json:"filename"`           // 文件名
	Error     string `json:"error,omitempty"`    // 错误信息（如果有）
	Segments  int    `json:"segments,omitempty"` // 段落数量（处理完成后）
	CreatedAt string `json:"created_at"`         // 创建时间
	UpdatedAt string `json:"updated_at"`         // 更新时间
}

// DocumentInfo 文档信息
type DocumentInfo struct {
	FileID     string                 `json:"file_id"`     // 文件ID
	FileName   string                 `json:"filename"`    // 文件名
	Status     string                 `json:"status"`      // 状态
	Tags       string                 `json:"tags"`        // 标签
	UploadTime time.Time              `json:"upload_time"` // 上传时间
	Segments   int                    `json:"segments"`    // 段落数量
	Metadata   map[string]interface{} `json:"metadata"`    // 元数据
}

// DocumentListResponse 文档列表响应
type DocumentListResponse struct {
	Total     int            `json:"total"`     // 总数量
	Page      int            `json:"page"`      // 当前页码
	PageSize  int            `json:"page_size"` // 每页大小
	Documents []DocumentInfo `json:"documents"` // 文档列表
}

// DocumentDeleteResponse 文档删除响应
type DocumentDeleteResponse struct {
	Success bool   `json:"success"` // 是否成功
	FileID  string `json:"file_id"` // 文件ID
}

// QASourceInfo 问答来源信息
type QASourceInfo struct {
	Text     string `json:"text"`     // 相关文本段落
	FileID   string `json:"file_id"`  // 文件ID
	FileName string `json:"filename"` // 文件名
	Position int    `json:"position"` // 段落位置
}

// QAResponse 问答响应
type QAResponse struct {
	Question string         `json:"question"` // 用户问题
	Answer   string         `json:"answer"`   // AI生成的回答
	Sources  []QASourceInfo `json:"sources"`  // 来源信息
}

// ConvertToSourceInfo 将向量数据库文档转换为来源信息
func ConvertToSourceInfo(docs []vectordb.Document) []QASourceInfo {
	if len(docs) == 0 {
		return []QASourceInfo{}
	}

	sources := make([]QASourceInfo, len(docs))
	for i, doc := range docs {
		sources[i] = QASourceInfo{
			Text:     doc.Text,
			FileID:   doc.FileID,
			FileName: doc.FileName,
			Position: doc.Position,
		}
	}
	return sources
}

// PaginationResponse 分页响应信息
type PaginationResponse struct {
	Total    int `json:"total"`     // 总记录数
	Page     int `json:"page"`      // 当前页码
	PageSize int `json:"page_size"` // 每页大小
}
