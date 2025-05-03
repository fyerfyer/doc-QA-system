package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/fyerfyer/doc-QA-system/api/model"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ChatHandler 处理聊天相关的API请求
type ChatHandler struct {
	chatService *services.ChatService // 聊天服务
	qaService   *services.QAService   // 问答服务
	logger      *logrus.Logger        // 日志记录器
}

// NewChatHandler 创建新的聊天处理器
func NewChatHandler(chatService *services.ChatService, qaService *services.QAService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		qaService:   qaService,
		logger:      middleware.GetLogger(),
	}
}

// CreateChat 创建新的聊天会话
// POST /api/chats
func (h *ChatHandler) CreateChat(c *gin.Context) {
	// 绑定请求参数
	var req model.CreateChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid create chat request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 创建聊天会话
	session, err := h.chatService.CreateChat(c.Request.Context(), req.Title)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create chat session")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"创建聊天会话失败",
		))
		return
	}

	// 构建响应
	resp := model.CreateChatResponse{
		ChatID:    session.ID,
		Title:     session.Title,
		CreatedAt: session.CreatedAt,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// GetChatHistory 获取聊天历史记录
// GET /api/chats/:session_id
func (h *ChatHandler) GetChatHistory(c *gin.Context) {
	// 绑定路径参数
	var req model.GetChatHistoryRequest
	if err := c.ShouldBindUri(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid chat history request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的会话ID",
		))
		return
	}

	// 计算分页参数
	offset := (req.GetPage() - 1) * req.GetPageSize()
	limit := req.GetPageSize()

	// 获取聊天会话
	session, err := h.chatService.GetChatSession(c.Request.Context(), req.SessionID)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to get chat session")
		c.JSON(http.StatusNotFound, model.NewErrorResponse(
			http.StatusNotFound,
			"聊天会话不存在",
		))
		return
	}

	// 获取消息列表
	messages, _, err := h.chatService.GetChatMessages(c.Request.Context(), req.SessionID, offset, limit)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to get chat messages")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取聊天消息失败",
		))
		return
	}

	// 转换为响应格式
	messageInfos := make([]model.MessageInfo, 0, len(messages))
	for _, msg := range messages {
		// 处理消息中的引用来源（如果有）
		var sources []model.QASourceInfo
		if len(msg.Sources) > 0 {
			// 解析Sources字段
			var msgSources []models.Source
			// 使用标准json包进行解析，而不是直接调用方法
			if err := json.Unmarshal(msg.Sources, &msgSources); err == nil {
				for _, src := range msgSources {
					sources = append(sources, model.QASourceInfo{
						FileID:   src.FileID,
						FileName: src.FileName,
						Text:     src.Text,
						Position: src.Position,
					})
				}
			}
		}

		messageInfos = append(messageInfos, model.MessageInfo{
			ID:        strconv.Itoa(int(msg.ID)),
			Role:      string(msg.Role),
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt,
			Sources:   sources,
		})
	}

	// 构建响应
	resp := model.ChatHistoryResponse{
		ChatID:   session.ID,
		Title:    session.Title,
		Messages: messageInfos,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// ListChats 获取聊天会话列表
// GET /api/chats
func (h *ChatHandler) ListChats(c *gin.Context) {
	// 绑定查询参数
	var req model.ChatListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid chat list request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 计算分页参数
	offset := (req.GetPage() - 1) * req.GetPageSize()
	limit := req.GetPageSize()

	// 构建过滤条件
	filters := make(map[string]interface{})
	if req.Tags != "" {
		filters["tags"] = req.Tags
	}
	if req.StartTime != nil {
		filters["start_time"] = *req.StartTime
	}
	if req.EndTime != nil {
		filters["end_time"] = *req.EndTime
	}

	// 获取带有消息数量的聊天列表
	chats, total, err := h.chatService.GetChatsWithMessageCount(c.Request.Context(), offset, limit)
	if err != nil {
		h.logger.WithError(err).Error("Failed to list chat sessions")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取聊天会话列表失败",
		))
		return
	}

	// 转换为响应格式
	chatInfos := make([]model.ChatInfo, 0, len(chats))
	for _, chat := range chats {
		chatInfos = append(chatInfos, model.ChatInfo{
			ID:           chat["id"].(string),
			Title:        chat["title"].(string),
			CreatedAt:    chat["created_at"].(time.Time),
			UpdatedAt:    chat["updated_at"].(time.Time),
			MessageCount: int(chat["message_count"].(int64)),
		})
	}

	// 构建响应
	resp := model.ChatListResponse{
		Total:    total,
		Page:     req.GetPage(),
		PageSize: req.GetPageSize(),
		Chats:    chatInfos,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// AddMessage 向聊天会话添加消息
// POST /api/chats/messages
func (h *ChatHandler) AddMessage(c *gin.Context) {
	// 绑定请求参数
	var req model.CreateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid add message request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 检查会话是否存在
	_, err := h.chatService.GetChatSession(c.Request.Context(), req.SessionID)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Chat session not found")
		c.JSON(http.StatusNotFound, model.NewErrorResponse(
			http.StatusNotFound,
			"聊天会话不存在",
		))
		return
	}

	// 创建消息对象
	message := &models.ChatMessage{
		SessionID: req.SessionID,
		Role:      models.MessageRole(req.Role),
		Content:   req.Content,
	}

	// 如果是用户消息，生成助手回复
	if message.Role == models.RoleUser {
		// 添加用户消息
		if err := h.chatService.AddMessage(c.Request.Context(), message); err != nil {
			h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to add user message")
			c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
				http.StatusInternalServerError,
				"添加用户消息失败",
			))
			return
		}

		// 使用QA服务生成回答
		answer, sources, err := h.qaService.Answer(c.Request.Context(), req.Content)
		if err != nil {
			h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to generate answer")

			// 即使生成回答失败，也添加一条错误消息
			errMessage := &models.ChatMessage{
				SessionID: req.SessionID,
				Role:      models.RoleAssistant,
				Content:   "抱歉，我无法回答这个问题。" + err.Error(),
			}
			h.chatService.AddMessage(c.Request.Context(), errMessage)

			c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
				http.StatusInternalServerError,
				"生成回答失败",
			))
			return
		}

		// 转换引用来源为Source结构
		modelSources := make([]models.Source, 0, len(sources))
		for _, src := range sources {
			modelSources = append(modelSources, models.Source{
				FileID:   src.FileID,
				FileName: src.FileName,
				Position: src.Position,
				Text:     src.Text,
			})
		}

		// 添加助手回复消息
		assistantMessage := &models.ChatMessage{
			SessionID: req.SessionID,
			Role:      models.RoleAssistant,
			Content:   answer,
		}

		if err := h.chatService.SaveMessageWithSources(
			c.Request.Context(),
			assistantMessage,
			modelSources,
		); err != nil {
			h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to add assistant message")
			c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
				http.StatusInternalServerError,
				"添加助手回复失败",
			))
			return
		}

		// 获取最新的两条消息（用户问题和助手回复）
		messages, _, err := h.chatService.GetChatMessages(c.Request.Context(), req.SessionID, 0, 2)
		if err != nil || len(messages) < 2 {
			h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to get latest messages")
			c.JSON(http.StatusOK, model.NewSuccessResponse(map[string]interface{}{
				"success": true,
				"message": "消息已添加，但无法获取最新回复",
			}))
			return
		}

		// 构建回复消息的响应
		userMsg := messages[0]
		assistantMsg := messages[1]

		// 构建引用来源
		var responseSources []model.QASourceInfo
		for _, src := range modelSources {
			responseSources = append(responseSources, model.QASourceInfo{
				FileID:   src.FileID,
				FileName: src.FileName,
				Text:     src.Text,
				Position: src.Position,
			})
		}

		// 构建响应对象
		resp := map[string]interface{}{
			"success": true,
			"user_message": model.MessageInfo{
				ID:        strconv.Itoa(int(userMsg.ID)),
				Role:      string(userMsg.Role),
				Content:   userMsg.Content,
				CreatedAt: userMsg.CreatedAt,
			},
			"assistant_message": model.MessageInfo{
				ID:        strconv.Itoa(int(assistantMsg.ID)),
				Role:      string(assistantMsg.Role),
				Content:   assistantMsg.Content,
				CreatedAt: assistantMsg.CreatedAt,
				Sources:   responseSources,
			},
		}

		c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
		return
	}

	// 非用户消息直接添加（系统消息等）
	if err := h.chatService.AddMessage(c.Request.Context(), message); err != nil {
		h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to add message")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"添加消息失败",
		))
		return
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(map[string]interface{}{
		"success": true,
		"message": "消息已添加",
	}))
}

// DeleteChat 删除聊天会话
// DELETE /api/chats/:session_id
func (h *ChatHandler) DeleteChat(c *gin.Context) {
	// 绑定路径参数
	var req model.DeleteChatRequest
	if err := c.ShouldBindUri(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid delete chat request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的会话ID",
		))
		return
	}

	// 删除会话
	err := h.chatService.DeleteChatSession(c.Request.Context(), req.SessionID)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to delete chat session")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"删除聊天会话失败",
		))
		return
	}

	// 构建响应
	resp := model.DeleteChatResponse{
		Success:   true,
		SessionID: req.SessionID,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// RenameChat 重命名聊天会话
// PATCH /api/chats/:session_id
func (h *ChatHandler) RenameChat(c *gin.Context) {
	// 绑定路径参数
	var pathParams model.RenameChatRequest
	if err := c.ShouldBindUri(&pathParams); err != nil {
		h.logger.WithError(err).Warn("Invalid rename chat request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的会话ID",
		))
		return
	}

	// 绑定请求体
	var req model.RenameChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid rename chat request body")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 确保路径参数和请求体中的会话ID一致
	req.SessionID = pathParams.SessionID

	// 重命名会话
	if err := h.chatService.RenameChatSession(c.Request.Context(), req.SessionID, req.Title); err != nil {
		h.logger.WithError(err).
			WithFields(logrus.Fields{
				"session_id": req.SessionID,
				"new_title":  req.Title,
			}).
			Error("Failed to rename chat session")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"重命名聊天会话失败",
		))
		return
	}

	// 获取更新后的会话
	session, err := h.chatService.GetChatSession(c.Request.Context(), req.SessionID)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", req.SessionID).Error("Failed to get renamed chat session")
		c.JSON(http.StatusOK, model.NewSuccessResponse(map[string]interface{}{
			"success":   true,
			"sessionId": req.SessionID,
			"title":     req.Title,
		}))
		return
	}

	// 构建响应
	resp := map[string]interface{}{
		"success":    true,
		"session_id": session.ID,
		"title":      session.Title,
		"updated_at": session.UpdatedAt,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// GetRecentQuestions 获取最近的问题
// GET /api/chat/recent-questions
func (h *ChatHandler) GetRecentQuestions(c *gin.Context) {
	// 绑定查询参数
	var req model.GetRecentQuestionsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid recent questions request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 获取最近的问题
	questions, err := h.qaService.GetRecentQuestions(c.Request.Context(), req.Limit)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get recent questions")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"获取最近问题失败",
		))
		return
	}

	// 构建响应
	resp := model.GetRecentQuestionsResponse{
		Questions: questions,
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}

// CreateChatWithMessage 创建会话并添加第一条消息
// POST /api/chats/with-message
func (h *ChatHandler) CreateChatWithMessage(c *gin.Context) {
	// 绑定请求参数
	var req model.CreateChatWithMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithError(err).Warn("Invalid create chat with message request")
		c.JSON(http.StatusBadRequest, model.NewErrorResponse(
			http.StatusBadRequest,
			"无效的请求参数",
		))
		return
	}

	// 创建聊天会话
	session, err := h.chatService.CreateChat(c.Request.Context(), req.Title)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create chat session")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"创建聊天会话失败",
		))
		return
	}

	// 创建用户消息
	userMessage := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   req.Content,
	}

	// 添加用户消息
	if err := h.chatService.AddMessage(c.Request.Context(), userMessage); err != nil {
		h.logger.WithError(err).WithField("session_id", session.ID).Error("Failed to add user message")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"添加用户消息失败",
		))
		return
	}

	// 使用QA服务生成回答
	answer, sources, err := h.qaService.Answer(c.Request.Context(), req.Content)
	if err != nil {
		h.logger.WithError(err).WithField("session_id", session.ID).Error("Failed to generate answer")

		// 即使生成回答失败，也添加一条错误消息
		errMessage := &models.ChatMessage{
			SessionID: session.ID,
			Role:      models.RoleAssistant,
			Content:   "抱歉，我无法回答这个问题。" + err.Error(),
		}
		h.chatService.AddMessage(c.Request.Context(), errMessage)

		// 返回已创建的会话信息，但提示回答生成失败
		resp := model.CreateChatWithMessageResponse{
			SessionID: session.ID,
			Title:     session.Title,
			CreatedAt: session.CreatedAt,
			Message: model.ChatMessageResponse{
				Role:    string(models.RoleUser),
				Content: req.Content,
			},
		}

		c.JSON(http.StatusOK, model.NewSuccessResponse(map[string]interface{}{
			"session":       resp,
			"error_message": "创建会话成功，但生成回答失败",
		}))
		return
	}

	// 转换引用来源为Source结构
	modelSources := make([]models.Source, 0, len(sources))
	for _, src := range sources {
		modelSources = append(modelSources, models.Source{
			FileID:   src.FileID,
			FileName: src.FileName,
			Position: src.Position,
			Text:     src.Text,
		})
	}

	// 添加助手回复消息
	assistantMessage := &models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   answer,
	}

	if err := h.chatService.SaveMessageWithSources(
		c.Request.Context(),
		assistantMessage,
		modelSources,
	); err != nil {
		h.logger.WithError(err).WithField("session_id", session.ID).Error("Failed to add assistant message")
		c.JSON(http.StatusInternalServerError, model.NewErrorResponse(
			http.StatusInternalServerError,
			"添加助手回复失败",
		))
		return
	}

	// 获取最新的助手消息
	messages, _, err := h.chatService.GetChatMessages(c.Request.Context(), session.ID, 0, 2)
	if err != nil || len(messages) < 2 {
		h.logger.WithError(err).WithField("session_id", session.ID).Error("Failed to get latest messages")
		c.JSON(http.StatusOK, model.NewSuccessResponse(map[string]interface{}{
			"success":       true,
			"session_id":    session.ID,
			"title":         session.Title,
			"error_message": "会话创建成功，但无法获取最新消息",
		}))
		return
	}

	// 构建QA源信息
	var responseSources []model.QASourceInfo
	for _, src := range sources {
		responseSources = append(responseSources, model.QASourceInfo{
			FileID:   src.FileID,
			FileName: src.FileName,
			Text:     src.Text,
			Position: src.Position,
		})
	}

	// 获取助手消息
	assistantMsg := messages[1]

	resp := model.CreateChatWithMessageResponse{
		SessionID: session.ID,
		Title:     session.Title,
		CreatedAt: session.CreatedAt,
		Message: model.ChatMessageResponse{
			ID:        assistantMsg.ID,
			SessionID: session.ID,
			Role:      string(assistantMsg.Role),
			Content:   assistantMsg.Content,
			CreatedAt: assistantMsg.CreatedAt,
			Sources:   responseSources,
		},
	}

	c.JSON(http.StatusOK, model.NewSuccessResponse(resp))
}
