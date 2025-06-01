package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fyerfyer/doc-QA-system/api"
	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/fyerfyer/doc-QA-system/pkg/taskqueue"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	configPath  string
	showVersion bool
	devMode     bool
	logLevel    string
	version     = "1.0.0" // 版本号，可通过构建时传入
)

func init() {
	// 解析命令行参数
	flag.StringVar(&configPath, "config", "config.yaml", "Configuration file path")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&devMode, "dev", false, "Run in development mode")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// 显示版本信息
	if showVersion {
		fmt.Printf("DocQA System Version: %s\n", version)
		os.Exit(0)
	}
}

func main() {
	// 初始化日志
	logger := logrus.New()
	setLogLevel(logger, logLevel)

	// 开发模式下启用更详细的日志
	if devMode {
		logger.SetLevel(logrus.DebugLevel)
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	logger.Info("DocQA System starting...")

	// 加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	// 设置数据库
	err = setupDatabase(cfg, logger)
	if err != nil {
		logger.Fatalf("Failed to setup database: %v", err)
	}
	defer database.Close()

	// 创建存储服务
	fileStorage, err := createStorage(cfg.Storage)
	if err != nil {
		logger.Fatalf("Failed to create storage: %v", err)
	}

	// 创建向量数据库
	vectorDB, err := createVectorDB(cfg.VectorDB)
	if err != nil {
		logger.Fatalf("Failed to create vector database: %v", err)
	}
	defer vectorDB.Close()

	// 创建嵌入模型客户端
	embedClient, err := createEmbeddingClient(cfg.Embed)
	if err != nil {
		logger.Fatalf("Failed to create embedding client: %v", err)
	}

	// 创建大语言模型客户端
	llmClient, err := createLLMClient(cfg.LLM)
	if err != nil {
		logger.Fatalf("Failed to create LLM client: %v", err)
	}

	// 创建缓存服务
	cacheService, err := createCache(cfg.Cache)
	if err != nil {
		logger.Warnf("Failed to create cache, using in-memory cache: %v", err)
		cacheService, _ = cache.NewMemoryCache(cache.Config{
			DefaultTTL: time.Duration(cfg.Cache.TTL) * time.Second,
		})
	}

	// 创建RAG服务
	ragService := createRAGService(llmClient)

	// 创建文档仓储
	docRepo := repository.NewDocumentRepository()

	// 创建文档状态管理器
	statusManager := services.NewDocumentStatusManager(docRepo, logger)

	// 创建任务队列（如果启用了异步处理）
	var taskQueue taskqueue.Queue
	if cfg.Queue.Enable {
		taskQueue, err = setupTaskQueue(cfg.Queue, logger)
		if err != nil {
			logger.Fatalf("Failed to setup task queue: %v", err)
		}
		logger.Info("Task queue initialized successfully")
	}

	// 创建文档分段器配置
	splitterCfg := document.DefaultSplitterConfig()
	splitterCfg.ChunkSize = cfg.Document.ChunkSize
	splitterCfg.ChunkOverlap = cfg.Document.ChunkOverlap

	// 创建文档分段器
	splitter := document.NewTextSplitter(splitterCfg)

	// 创建文档服务
	documentService := services.NewDocumentService(
		fileStorage,
		nil, // 使用ParserFactory
		splitter,
		embedClient,
		vectorDB,
		services.WithLogger(logger),
		services.WithDocumentRepository(docRepo),
		services.WithStatusManager(statusManager),
		services.WithBatchSize(cfg.Embed.BatchSize),
	)

	// 如果启用了任务队列，则启用异步处理
	if cfg.Queue.Enable && taskQueue != nil {
		documentService.EnableAsyncProcessing(taskQueue)
		logger.Info("Async document processing enabled")
	}

	// 创建问答服务
	qaService := services.NewQAService(
		embedClient,
		vectorDB,
		llmClient,
		ragService,
		cacheService,
		services.WithCacheTTL(time.Duration(cfg.Cache.TTL)*time.Second),
		services.WithSearchLimit(cfg.Search.Limit),
		services.WithMinScore(cfg.Search.MinScore),
	)

	// 创建API处理器
	docHandler := handler.NewDocumentHandler(documentService, fileStorage)
	qaHandler := handler.NewQAHandler(qaService)

	// 设置路由
	router := api.SetupRouter(docHandler, qaHandler)

	// 注册任务回调路由
	if cfg.Queue.Enable {
		taskHandler := handler.NewTaskHandler(taskQueue)
		api.RegisterTaskRoutes(router, taskHandler)
		logger.Info("Task callback routes registered")
	}

	// 配置HTTP服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// 启动HTTP服务器
	go func() {
		logger.Infof("Server starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中断信号优雅关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// 设置关闭超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}

// 设置日志级别
func setLogLevel(logger *logrus.Logger, level string) {
	switch level {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}
}

// 设置数据库
func setupDatabase(cfg *config.Config, logger *logrus.Logger) error {
	// 默认使用SQLite
	dbConfig := &database.Config{
		Type: "sqlite",
		DSN:  "data/docqa.db", // 默认数据库路径
	}

	// 如果配置中指定了数据库设置，则使用配置中的设置
	if cfg.Database.Type != "" {
		dbConfig.Type = cfg.Database.Type
	}
	if cfg.Database.DSN != "" {
		dbConfig.DSN = cfg.Database.DSN
	}

	// 初始化数据库
	return database.Setup(dbConfig, logger)
}

// 创建存储服务
func createStorage(cfg config.StorageConfig) (storage.Storage, error) {
	switch cfg.Type {
	case "local":
		return storage.NewLocalStorage(storage.LocalConfig{
			Path: cfg.Path,
		})
	case "minio":
		return storage.NewMinioStorage(storage.MinioConfig{
			Endpoint:  cfg.Endpoint,
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
			UseSSL:    cfg.UseSSL,
			Bucket:    cfg.Bucket,
		})
	default:
		return storage.NewLocalStorage(storage.LocalConfig{
			Path: "./uploads",
		})
	}
}

// 创建向量数据库
func createVectorDB(cfg config.VectorDBConfig) (vectordb.Repository, error) {
	// 创建向量数据库配置
	vectorConfig := vectordb.Config{
		Type:              cfg.Type,
		Path:              cfg.Path,
		Dimension:         cfg.Dim,
		CreateIfNotExists: true,
	}

	// 设置距离计算方式
	switch cfg.Distance {
	case "cosine":
		vectorConfig.DistanceType = vectordb.Cosine
	case "l2":
		vectorConfig.DistanceType = vectordb.Euclidean
	case "dot":
		vectorConfig.DistanceType = vectordb.DotProduct
	default:
		vectorConfig.DistanceType = vectordb.Cosine
	}

	// 创建向量数据库
	return vectordb.NewRepository(vectorConfig)
}

// 创建嵌入模型客户端
func createEmbeddingClient(cfg config.EmbedConfig) (embedding.Client, error) {
	// 设置嵌入模型选项
	var opts []embedding.Option
	opts = append(opts, embedding.WithAPIKey(cfg.APIKey))

	if cfg.Endpoint != "" {
		opts = append(opts, embedding.WithBaseURL(cfg.Endpoint))
	}

	if cfg.Model != "" {
		opts = append(opts, embedding.WithModel(cfg.Model))
	}

	if cfg.BatchSize > 0 {
		opts = append(opts, embedding.WithBatchSize(cfg.BatchSize))
	}

	if cfg.Dimensions > 0 {
		opts = append(opts, embedding.WithDimensions(cfg.Dimensions))
	}

	// 根据提供商创建客户端
	switch cfg.Provider {
	case "tongyi", "dashscope":
		return embedding.NewClient("tongyi", opts...)
	case "openai":
		return embedding.NewClient("openai", opts...)
	case "local", "huggingface":
		return embedding.NewClient("huggingface", opts...)
	default:
		// 默认使用通义千问
		return embedding.NewClient("tongyi", opts...)
	}
}

// 创建大语言模型客户端
func createLLMClient(cfg config.LLMConfig) (llm.Client, error) {
	// 设置大模型选项
	var opts []llm.Option
	opts = append(opts, llm.WithAPIKey(cfg.APIKey))

	if cfg.Endpoint != "" {
		opts = append(opts, llm.WithBaseURL(cfg.Endpoint))
	}

	if cfg.Model != "" {
		opts = append(opts, llm.WithModel(cfg.Model))
	}

	if cfg.MaxTokens > 0 {
		opts = append(opts, llm.WithMaxTokens(cfg.MaxTokens))
	}

	if cfg.Temperature > 0 {
		opts = append(opts, llm.WithTemperature(cfg.Temperature))
	}

	// 根据提供商创建客户端
	switch cfg.Provider {
	case "tongyi", "dashscope":
		return llm.NewClient("tongyi", opts...)
	case "openai":
		return llm.NewClient("openai", opts...)
	default:
		// 默认使用通义千问
		return llm.NewClient("tongyi", opts...)
	}
}

// 创建缓存服务
func createCache(cfg config.CacheConfig) (cache.Cache, error) {
	if !cfg.Enable {
		return cache.NewMemoryCache(cache.Config{
			DefaultTTL: time.Duration(cfg.TTL) * time.Second,
		})
	}

	cacheConfig := cache.Config{
		Type:          cfg.Type,
		RedisAddr:     cfg.Address,
		RedisPassword: cfg.Password,
		RedisDB:       cfg.DB,
		DefaultTTL:    time.Duration(cfg.TTL) * time.Second,
	}

	return cache.NewCache(cacheConfig)
}

// 创建RAG服务
func createRAGService(llmClient llm.Client) *llm.RAGService {
	return llm.NewRAG(
		llmClient,
		llm.WithRAGMaxTokens(2048),
		llm.WithRAGTemperature(0.7),
	)
}

// 设置任务队列
func setupTaskQueue(cfg config.QueueConfig, logger *logrus.Logger) (taskqueue.Queue, error) {
	// 创建任务队列配置
	queueConfig := &taskqueue.Config{
		RedisAddr:     cfg.RedisAddr,
		RedisPassword: cfg.RedisPassword,
		RedisDB:       cfg.RedisDB,
		Concurrency:   cfg.Concurrency,
		RetryLimit:    cfg.RetryLimit,
		RetryDelay:    time.Duration(cfg.RetryDelay) * time.Second,
	}

	// 创建任务队列
	var queue taskqueue.Queue
	var err error

	switch cfg.Type {
	case "redis":
		queue, err = taskqueue.NewRedisQueue(queueConfig)
	default:
		// 默认使用Redis
		queue, err = taskqueue.NewRedisQueue(queueConfig)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create task queue: %w", err)
	}

	// 注册回调处理器
	processor := taskqueue.NewCallbackProcessor(queue, logger)
	processor.RegisterDefaultHandlers(queue)

	return queue, nil
}
