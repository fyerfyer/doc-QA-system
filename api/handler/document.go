package handler

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/models"
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

	// Debug logging for tags and content type
	h.logger.WithFields(logrus.Fields{
		"tags":         req.Tags,
		"content_type": c.Request.Header.Get("Content-Type"),
	}).Debug("Document upload request received with tags")

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

	// 通过状态管理器记录文档上传状态
	ctx := context.Background()
	if err := h.documentService.Init(); err == nil {
		docStatusManager := h.documentService.GetStatusManager()
		if docStatusManager != nil {
			// Pass the tags from the request to MarkAsUploaded
			err := docStatusManager.MarkAsUploaded(ctx, fileInfo.ID, filename, fileInfo.Path, fileInfo.Size)
			if err != nil {
				h.logger.WithError(err).Warn("Failed to mark document as uploaded")
			}

			// 更新文档标签
			if req.Tags != "" {
				doc, err := docStatusManager.GetDocument(ctx, fileInfo.ID)
				if err == nil {
					doc.Tags = req.Tags
					docStatusManager.GetRepo().Update(doc)
					h.logger.WithFields(logrus.Fields{
						"file_id": fileInfo.ID,
						"tags":    req.Tags,
					}).Debug("Updated document tags")
				}
			}
		}
	}

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
			// 状态更新由ProcessDocument内部处理
		} else {
			h.logger.WithField("file_id", fileInfo.ID).Info("Document processed successfully")
			// 状态更新由ProcessDocument内部处理
		}
	}()

	// 返回文件ID和状态
	resp := model.DocumentUploadResponse{
		FileID:   fileInfo.ID,
		FileName: filename,
		Status:   "uploaded", // 初始状态为已上传
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

	h.logger.WithFields(logrus.Fields{
		"status_type":  fmt.Sprintf("%T", docInfo["status"]),
		"status_value": fmt.Sprintf("%v", docInfo["status"]),
	}).Debug("Document status type information")

	h.logger.WithFields(logrus.Fields{
		"doc_id":       req.ID,
		"raw_doc_info": fmt.Sprintf("%+v", docInfo),
		"tags_field":   docInfo["tags"],
	}).Debug("Retrieved document info")

	// Fix the type conversion
	var statusStr string
	switch status := docInfo["status"].(type) {
	case models.DocumentStatus:
		statusStr = string(status)
	case string:
		statusStr = status
	default:
		statusStr = fmt.Sprintf("%v", status)
	}

	// 构建响应
	resp := model.DocumentStatusResponse{
		FileID:    req.ID,
		Status:    statusStr,
		FileName:  docInfo["filename"].(string),
		Segments:  segments,
		CreatedAt: docInfo["created_at"].(string),
		UpdatedAt: docInfo["updated_at"].(string),
	}

	// 如果有错误信息，添加到响应中
	if errMsg, ok := docInfo["error"]; ok {
		resp.Error = errMsg.(string)
	}

	// 如果有处理进度，添加到响应中
	if progress, ok := docInfo["progress"]; ok {
		resp.Progress = progress.(int)
	}

	// 如果有文件大小，添加到响应中
	if size, ok := docInfo["size"]; ok {
		if sizeInt, ok := size.(int64); ok {
			resp.Size = sizeInt
		}
	}

	// 如果有标签，添加到响应中
	if tags, ok := docInfo["tags"]; ok && tags.(string) != "" {
		resp.Tags = tags.(string)
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

	// 计算分页参数
	offset := (req.GetPage() - 1) * req.GetPageSize()
	limit := req.GetPageSize()

	// 构建过滤条件
	filters := make(map[string]interface{})

	if req.Status != "" {
		filters["status"] = req.Status
	}

	if req.Tags != "" {
		filters["tags"] = req.Tags
	}

	if req.StartTime != nil {
		filters["start_time"] = req.StartTime.Format(time.RFC3339)
	}

	if req.EndTime != nil {
		filters["end_time"] = req.EndTime.Format(time.RFC3339)
	}

	// 查询文档列表
	docs, total, err := h.documentService.ListDocuments(c.Request.Context(), offset, limit, filters)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":  err.Error(),
			"offset": offset,
			"limit":  limit,
		}).Error("Failed to fetch document list")

		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取文档列表失败: "+err.Error(),
		))
		return
	}

	// 转换为响应格式
	docInfos := make([]model.DocumentInfo, 0, len(docs))
	for _, doc := range docs {
		// 获取段落数量
		segments := doc.SegmentCount

		docInfo := model.DocumentInfo{
			FileID:     doc.ID,
			FileName:   doc.FileName,
			Status:     string(doc.Status),
			Tags:       doc.Tags,
			UploadTime: doc.UploadedAt,
			UpdatedAt:  doc.UpdatedAt,
			Segments:   segments,
			Size:       doc.FileSize,
			Progress:   doc.Progress,
		}

		docInfos = append(docInfos, docInfo)
	}

	// 构建分页响应
	resp := model.DocumentListResponse{
		Total:     total,
		Page:      req.GetPage(),
		PageSize:  req.GetPageSize(),
		Documents: docInfos,
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

// UpdateDocument 更新文档信息
// PATCH /api/documents/:id
func (h *DocumentHandler) UpdateDocument(c *gin.Context) {
	// 绑定路径参数
	var pathParams struct {
		ID string `uri:"id" binding:"required"`
	}
	if err := c.ShouldBindUri(&pathParams); err != nil {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(http.StatusBadRequest, "无效的文档ID"))
		return
	}

	// 绑定请求体
	var req struct {
		Tags string `json:"tags" binding:"omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求数据",
		))
		return
	}

	// 更新文档标签
	if req.Tags != "" {
		if err := h.documentService.UpdateDocumentTags(c.Request.Context(), pathParams.ID, req.Tags); err != nil {
			h.logger.WithFields(logrus.Fields{
				"error":   err.Error(),
				"file_id": pathParams.ID,
			}).Error("Failed to update document tags")

			c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
				http.StatusInternalServerError,
				"更新文档标签失败",
			))
			return
		}
	}

	// 获取最新的文档信息
	docInfo, err := h.documentService.GetDocumentInfo(c.Request.Context(), pathParams.ID)
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"file_id": pathParams.ID,
		}).Error("Failed to get updated document info")

		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取更新后的文档信息失败",
		))
		return
	}

	// 修复类型转换问题
	var statusStr string
	switch status := docInfo["status"].(type) {
	case models.DocumentStatus:
		statusStr = string(status)
	case string:
		statusStr = status
	default:
		statusStr = fmt.Sprintf("%v", status)
	}

	// 返回更新成功的响应
	resp := model.DocumentUpdateResponse{
		Success:  true,
		FileID:   pathParams.ID,
		FileName: docInfo["filename"].(string),
		Status:   statusStr,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// GetDocumentMetrics 获取文档统计信息
// GET /api/documents/metrics
func (h *DocumentHandler) GetDocumentMetrics(c *gin.Context) {
	// 获取各状态的文档计数
	ctx := c.Request.Context()

	// 获取上传状态的文档
	uploadedFilter := map[string]interface{}{"status": models.DocStatusUploaded}
	_, uploadedCount, _ := h.documentService.ListDocuments(ctx, 0, 0, uploadedFilter)

	// 获取处理中的文档
	processingFilter := map[string]interface{}{"status": models.DocStatusProcessing}
	_, processingCount, _ := h.documentService.ListDocuments(ctx, 0, 0, processingFilter)

	// 获取完成的文档
	completedFilter := map[string]interface{}{"status": models.DocStatusCompleted}
	_, completedCount, _ := h.documentService.ListDocuments(ctx, 0, 0, completedFilter)

	// 获取失败的文档
	failedFilter := map[string]interface{}{"status": models.DocStatusFailed}
	_, failedCount, _ := h.documentService.ListDocuments(ctx, 0, 0, failedFilter)

	// 获取所有文档
	_, totalCount, _ := h.documentService.ListDocuments(ctx, 0, 0, nil)

	// 构建并返回响应
	metrics := map[string]interface{}{
		"total":      totalCount,
		"uploaded":   uploadedCount,
		"processing": processingCount,
		"completed":  completedCount,
		"failed":     failedCount,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(metrics))
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
