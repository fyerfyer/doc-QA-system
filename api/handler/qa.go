package handler

import (
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"net/http"

	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// QAHandler 处理问答相关的API请求
type QAHandler struct {
	qaService *services.QAService // 问答服务
	logger    *logrus.Logger      // 日志记录器
}

// NewQAHandler 创建新的问答处理器
func NewQAHandler(qaService *services.QAService) *QAHandler {
	return &QAHandler{
		qaService: qaService,
		logger:    middleware.GetLogger(),
	}
}

// AnswerQuestion 处理问答请求
// POST /api/qa
func (h *QAHandler) AnswerQuestion(c *gin.Context) {
	// 绑定请求参数
	var req model.QARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Warn("Invalid question request")

		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 检查问题是否为空
	if req.Question == "" {
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"问题不能为空",
		))
		return
	}

	var answer string
	var sources []model.QASourceInfo

	// 根据请求类型选择不同的处理方式
	var err error
	ctx := c.Request.Context()

	if req.FileID != "" {
		// 从特定文件回答问题
		h.logger.WithFields(logrus.Fields{
			"question": req.Question,
			"file_id":  req.FileID,
		}).Info("Question with specific file")

		var sourceDocs []vectordb.Document
		answer, sourceDocs, err = h.qaService.AnswerWithFile(ctx, req.Question, req.FileID)
		if err == nil {
			sources = model.ConvertToSourceInfo(sourceDocs)
		}
	} else if len(req.Metadata) > 0 {
		// 使用元数据过滤回答问题
		h.logger.WithFields(logrus.Fields{
			"question": req.Question,
			"metadata": req.Metadata,
		}).Info("Question with metadata filter")

		var sourceDocs []vectordb.Document
		answer, sourceDocs, err = h.qaService.AnswerWithMetadata(ctx, req.Question, req.Metadata)
		if err == nil {
			sources = model.ConvertToSourceInfo(sourceDocs)
		}
	} else {
		// 普通问答
		h.logger.WithField("question", req.Question).Info("General question")

		var sourceDocs []vectordb.Document
		answer, sourceDocs, err = h.qaService.Answer(ctx, req.Question)
		if err == nil {
			sources = model.ConvertToSourceInfo(sourceDocs)
		}
	}

	// 处理错误
	if err != nil {
		h.logger.WithFields(logrus.Fields{
			"error":    err.Error(),
			"question": req.Question,
		}).Error("Failed to answer question")

		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"处理问题时出错: "+err.Error(),
		))
		return
	}

	// 构建响应
	resp := model.QAResponse{
		Question: req.Question,
		Answer:   answer,
		Sources:  sources,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}
