package taskqueue

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DocumentUpdate 文档更新消息结构
type DocumentUpdate struct {
	DocumentID   string `json:"document_id"`             // 文档ID
	Status       string `json:"status"`                  // 状态
	SegmentCount int    `json:"segment_count,omitempty"` // 段落数量
	Error        string `json:"error,omitempty"`         // 错误信息
}

// DocumentStatusUpdater 文档状态更新接口
// 实际项目中可能由 services.DocumentStatusManager 实现
type DocumentStatusUpdater interface {
	// MarkAsProcessing 标记文档为处理中
	MarkAsProcessing(ctx context.Context, documentID string) error

	// MarkAsCompleted 标记文档为处理完成
	MarkAsCompleted(ctx context.Context, documentID string, segmentCount int) error

	// MarkAsFailed 标记文档为处理失败
	MarkAsFailed(ctx context.Context, documentID string, errorMsg string) error
}

// UpdateListener 更新监听器
// 用于监听各种状态更新消息
type UpdateListener struct {
	client          *redis.Client         // Redis客户端
	documentUpdater DocumentStatusUpdater // 文档状态更新器
	ctx             context.Context       // 上下文
	cancel          context.CancelFunc    // 取消函数
	wg              sync.WaitGroup        // 等待组
	channels        []string              // 监听的频道列表
}

// UpdateListenerOption 监听器配置选项
type UpdateListenerOption func(*UpdateListener)

// WithChannel 添加监听频道
func WithChannel(channel string) UpdateListenerOption {
	return func(l *UpdateListener) {
		l.channels = append(l.channels, channel)
	}
}

// WithDocumentUpdater 设置文档状态更新器
func WithDocumentUpdater(updater DocumentStatusUpdater) UpdateListenerOption {
	return func(l *UpdateListener) {
		l.documentUpdater = updater
	}
}

// NewUpdateListener 创建更新监听器
func NewUpdateListener(redisClient *redis.Client, options ...UpdateListenerOption) *UpdateListener {
	ctx, cancel := context.WithCancel(context.Background())

	listener := &UpdateListener{
		client:   redisClient,
		ctx:      ctx,
		cancel:   cancel,
		channels: []string{"document_updates"}, // 默认监听文档更新频道
	}

	// 应用配置选项
	for _, option := range options {
		option(listener)
	}

	return listener
}

// SetDocumentUpdater 设置文档状态更新器
// 这个方法允许在监听器创建后更改文档状态更新器
func (l *UpdateListener) SetDocumentUpdater(updater DocumentStatusUpdater) {
	l.documentUpdater = updater
}

// Start 开始监听
func (l *UpdateListener) Start() {
	l.wg.Add(1)
	go l.listen()
	log.Println("Update listener started")
}

// Stop 停止监听
func (l *UpdateListener) Stop() {
	l.cancel()
	l.wg.Wait()
	log.Println("Update listener stopped")
}

// listen 监听Redis消息
func (l *UpdateListener) listen() {
	defer l.wg.Done()

	// 订阅频道
	pubsub := l.client.Subscribe(l.ctx, l.channels...)
	defer pubsub.Close()

	// 确认订阅成功
	_, err := pubsub.Receive(l.ctx)
	if err != nil {
		log.Printf("Failed to subscribe to channels: %v", err)
		return
	}

	// 获取消息通道
	ch := pubsub.Channel()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return // 通道已关闭
			}
			l.handleMessage(msg)
		case <-l.ctx.Done():
			return // 上下文已取消
		}
	}
}

// handleMessage 处理接收到的消息
func (l *UpdateListener) handleMessage(msg *redis.Message) {
	// 根据频道类型分发处理
	switch msg.Channel {
	case "document_updates":
		l.handleDocumentUpdate(msg.Payload)
	default:
		log.Printf("Received message on unknown channel %s", msg.Channel)
	}
}

// handleDocumentUpdate 处理文档更新消息
func (l *UpdateListener) handleDocumentUpdate(payload string) {
	// 如果没有文档更新器，则忽略消息
	if l.documentUpdater == nil {
		log.Println("Document updater not configured, ignoring document update")
		return
	}

	// 解析JSON消息
	var update DocumentUpdate
	if err := json.Unmarshal([]byte(payload), &update); err != nil {
		log.Printf("Failed to parse document update: %v", err)
		return
	}

	// 创建请求上下文
	ctx, cancel := context.WithTimeout(l.ctx, defaultTimeout)
	defer cancel()

	// 根据状态调用相应的更新方法
	var err error
	switch update.Status {
	case "processing":
		err = l.documentUpdater.MarkAsProcessing(ctx, update.DocumentID)
	case "completed":
		err = l.documentUpdater.MarkAsCompleted(ctx, update.DocumentID, update.SegmentCount)
	case "failed":
		err = l.documentUpdater.MarkAsFailed(ctx, update.DocumentID, update.Error)
	default:
		log.Printf("Unknown document status: %s", update.Status)
		return
	}

	// 处理错误
	if err != nil {
		log.Printf("Failed to update document status: %v", err)
	}
}

// defaultTimeout 是操作的默认超时时间
const defaultTimeout = 30 * time.Second

// 以下是一些辅助方法，用于快速创建配置好的监听器

// NewDocumentUpdateListener 创建专门用于文档更新的监听器
func NewDocumentUpdateListener(redisClient *redis.Client, documentUpdater DocumentStatusUpdater) *UpdateListener {
	return NewUpdateListener(
		redisClient,
		WithDocumentUpdater(documentUpdater),
		WithChannel("document_updates"),
	)
}
