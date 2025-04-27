package handler

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// DocumentHandler 处理文档相关的API请求
type DocumentHandler struct {
	documentService *services.DocumentService // 文档服务
	fileStorage     storage.Storage           // 文件存储服务
	logger          *logrus.Logger            // 日志记录器
}

// NewDocumentHandler 创建新的文档处理器
func NewDocumentHandler(documentService *services.DocumentService, fileStorage storage.Storage) *DocumentHandler {
	return &DocumentHandler{
		documentService: documentService,
		fileStorage:     fileStorage,
		logger:          middleware.GetLogger(),
	}
}

// UploadDocument 处理文档上传请求
// POST /api/documents
func (h *DocumentHandler) UploadDocument(c *gin.Context) {
	// 绑定请求参数
	var req model.DocumentUploadRequest
	if err := c.ShouldBind(&req); err != nil {
		h.logger.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Warn("Invalid document upload request")

		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 检查文件
	if req.File == nil {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"未提供文件",
		))
		return
	}

	// 检查文件类型
	filename := req.File.Filename
	ext := filepath.Ext(filename)
	if !isValidFileType(ext) {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"不支持的文件类型，仅支持 .pdf, .md, .markdown, .txt",
		))
		return
	}

	// 打开上传的文件
	file, err := req.File.Open()
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":    err.Error(),
			"filename": filename,
		}).Error("Failed to open uploaded file")

		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"无法打开上传的文件",
		))
		return
	}
	defer file.Close()

	// 保存文件到存储
	fileInfo, err := h.fileStorage.Save(file, filename)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":    err.Error(),
			"filename": filename,
		}).Error("Failed to save file")

		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"保存文件失败",
		))
		return
	}

	// 记录文件上传信息
	h.logger.WithFields(logrus.Fields{
		"file_id":  fileInfo.ID,
		"filename": fileInfo.Name,
		"path":     fileInfo.Path,
		"size":     fileInfo.Size,
	}).Info("File uploaded successfully")

	// 启动异步处理任务
	go func() {
		// 记录开始处理
		h.logger.WithField("file_id", fileInfo.ID).Info("Starting document processing")
		ctx := context.Background()

		if err := h.documentService.ProcessDocument(ctx, fileInfo.ID, fileInfo.Path); err != nil {
			h.logger.WithFields(logrus.Fields{
				"error":   err.Error(),
				"file_id": fileInfo.ID,
			}).Error("Failed to process document")

			// TODO: 更新文档状态为失败
		} else {
			h.logger.WithField("file_id", fileInfo.ID).Info("Document processed successfully")
			// TODO: 更新文档状态为完成
		}
	}()

	// 返回文件ID和状态
	resp := model.DocumentUploadResponse{
		FileID:   fileInfo.ID,
		FileName: filename,
		Status:   "processing", // 初始状态为处理中
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// GetDocumentStatus 获取文档处理状态
// GET /api/documents/:id/status
func (h *DocumentHandler) GetDocumentStatus(c *gin.Context) {
	// 绑定路径参数
	var req model.DocumentStatusRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(http.StatusBadRequest, "无效的文档ID"))
		return
	}

	// 获取文档信息
	docInfo, err := h.documentService.GetDocumentInfo(c.Request.Context(), req.ID)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"file_id": req.ID,
		}).Error("Failed to get document info")

		c.JSON(http.StatusNotFound, model.NewErrorResponse(http.StatusNotFound, "未找到文档或获取信息失败"))
		return
	}

	// 获取段落数量
	segments, err := h.documentService.CountDocumentSegments(c.Request.Context(), req.ID)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"file_id": req.ID,
		}).Warn("Failed to count document segments")
		// 继续处理，不返回错误
	}

	// 构建响应
	resp := model.DocumentStatusResponse{
		FileID:    req.ID,
		Status:    docInfo["status"].(string),
		FileName:  docInfo["filename"].(string),
		Segments:  segments,
		CreatedAt: docInfo["created_at"].(string),
		UpdatedAt: docInfo["updated_at"].(string),
	}

	// 如果有错误信息，添加到响应中
	if errMsg, ok := docInfo["error"]; ok {
		resp.Error = errMsg.(string)
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// ListDocuments 获取文档列表
// GET /api/documents
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	// 绑定查询参数
	var req model.DocumentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(http.StatusBadRequest, "无效的查询参数"))
		return
	}

	// 构建过滤条件
	filterOptions := make(map[string]interface{})

	if req.Status != "" {
		filterOptions["status"] = req.Status
	}

	if req.Tags != "" {
		filterOptions["tags"] = req.Tags
	}

	if req.StartTime != nil {
		filterOptions["start_time"] = req.StartTime.Format(time.RFC3339)
	}

	if req.EndTime != nil {
		filterOptions["end_time"] = req.EndTime.Format(time.RFC3339)
	}

	// TODO: 实现文档列表查询
	// 这里需要DocumentService提供一个ListDocuments方法
	// 由于这是一个待实现的功能，我们先返回一个空列表

	h.logger.Info("Document list requested, returning empty list (feature not implemented yet)")

	// 构建分页响应
	resp := model.DocumentListResponse{
		Total:     0,
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		Documents: []model.DocumentInfo{},
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// DeleteDocument 删除文档
// DELETE /api/documents/:id
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	// 绑定路径参数
	var req model.DocumentDeleteRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(http.StatusBadRequest, "无效的文档ID"))
		return
	}

	// 删除文档
	err := h.documentService.DeleteDocument(c.Request.Context(), req.ID)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"file_id": req.ID,
		}).Error("Failed to delete document")

		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"删除文档失败",
		))
		return
	}

	h.logger.WithField("file_id", req.ID).Info("Document deleted successfully")

	// 返回成功响应
	resp := model.DocumentDeleteResponse{
		Success: true,
		FileID:  req.ID,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// isValidFileType 检查文件类型是否有效
func isValidFileType(ext string) bool {
	validTypes := map[string]bool{
		".pdf":      true,
		".md":       true,
		".markdown": true,
		".txt":      true,
	}
	return validTypes[ext]
}
