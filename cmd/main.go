package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fyerfyer/doc-QA-system/api"
	"github.com/fyerfyer/doc-QA-system/api/middleware"

	"github.com/fyerfyer/doc-QA-system/api/handler"
	qaconfig "github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/document"
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

// 配置选项
type config struct {
	Port            int           // 服务端口
	Mode            string        // 运行模式 (debug/release)
	StoragePath     string        // 文件存储路径
	VectorDBPath    string        // 向量数据库路径
	VectorDimension int           // 向量维度
	EmbeddingModel  string        // 嵌入模型名称
	EmbeddingAPIKey string        // 嵌入API密钥
	LLMModel        string        // 大语言模型名称
	LLMAPIKey       string        // 大语言模型API密钥
	MaxChunkSize    int           // 最大文本块大小
	CacheType       string        // 缓存类型
	LogLevel        string        // 日志级别
	ReadTimeout     time.Duration // 读取超时
	WriteTimeout    time.Duration // 写入超时
	DataDir         string        // 数据目录路径
	ConfigFile      string        // 配置文件路径
	// 任务队列相关配置
	QueueEnabled     bool          // 是否启用任务队列
	QueueType        string        // 任务队列类型
	RedisAddr        string        // Redis 地址
	RedisPassword    string        // Redis 密码
	RedisDB          int           // Redis 数据库编号
	QueueConcurrency int           // 任务队列处理并发数
	QueueRetryLimit  int           // 任务重试次数
	QueueRetryDelay  time.Duration // 任务重试延迟
}

func main() {
	// 解析命令行参数
	cfg := parseFlags()

	// 加载配置文件(如果指定)
	var appConfig *qaconfig.Config
	var err error
	if cfg.ConfigFile != "" {
		appConfig, err = qaconfig.Load(cfg.ConfigFile)
		if err != nil {
			log.Printf("Warning: Failed to load config file: %v, using command line args", err)
		} else {
			// 使用配置文件中的值更新相关设置
			updateConfigFromFile(&cfg, appConfig)
		}
	}

	// 设置Gin模式
	gin.SetMode(cfg.Mode)

	// 初始化日志
	logger := setupLogger(cfg.LogLevel)
	logger.Info("Starting Document QA System...")

	// 初始化数据库
	if err := setupDatabase(cfg, logger); err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}

	// 创建文件存储服务
	fileStorage, err := setupStorage(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize storage: %v", err)
	}

	// 创建向量数据库
	vectorDB, err := setupVectorDB(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize vector database: %v", err)
	}
	defer vectorDB.Close()

	// 创建嵌入客户端
	embeddingClient, err := setupEmbedding(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize embedding client: %v", err)
	}

	// 创建大语言模型客户端
	llmClient, err := setupLLM(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize LLM client: %v", err)
	}

	// 创建缓存服务
	cacheService, err := setupCache(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize cache: %v", err)
	}

	// 初始化任务队列（如果启用）
	var queue taskqueue.Queue
	if cfg.QueueEnabled {
		queue, err = setupTaskQueue(cfg, logger)
		if err != nil {
			logger.Fatalf("Failed to initialize task queue: %v", err)
		}
		defer queue.Close()
		logger.Info("Task queue initialized successfully")
	}

	// 创建文本分段器
	splitter := document.NewTextSplitter(document.SplitterConfig{
		SplitType:    document.ByParagraph,
		ChunkSize:    2000, // 从1000增加到2000
		ChunkOverlap: 400,  // 从200增加到400
	})

	// 初始化RAG服务
	ragService := llm.NewRAG(llmClient,
		llm.WithRAGMaxTokens(2048),
		llm.WithRAGTemperature(0.7),
	)

	// 初始化业务服务
	var repo repository.DocumentRepository
	if queue != nil {
		// 如果启用了任务队列，使用带队列的仓储
		repo = repository.NewDocumentRepositoryWithQueue(database.MustDB(), queue)
		logger.Info("Using document repository with task queue")
	} else {
		repo = repository.NewDocumentRepository()
	}

	statusManager := services.NewDocumentStatusManager(repo, logger)

	// 创建文档服务，根据是否启用队列进行配置
	documentServiceOptions := []services.DocumentOption{
		services.WithDocumentRepository(repo),
		services.WithStatusManager(statusManager),
		services.WithBatchSize(16),
		services.WithLogger(logger),
	}

	// 如果启用了队列，添加相关选项
	if queue != nil {
		documentServiceOptions = append(documentServiceOptions,
			services.WithTaskQueue(queue),
			services.WithAsyncProcessing(true),
		)
		logger.Info("Document processing will use async task queue")
	}

	documentService := services.NewDocumentService(
		fileStorage,
		nil, // document.Parser通过调用ParserFactory动态创建
		splitter,
		embeddingClient,
		vectorDB,
		documentServiceOptions...,
	)

	qaService := services.NewQAService(
		embeddingClient,
		vectorDB,
		llmClient,
		ragService,
		cacheService,
		services.WithMinScore(0.5),  // 将阈值从0.7降低到0.5
		services.WithSearchLimit(8), // 从默认的5增加到8
	)

	// 初始化API处理器
	docHandler := handler.NewDocumentHandler(documentService, fileStorage)
	qaHandler := handler.NewQAHandler(qaService)

	// 设置路由
	r := api.SetupRouter(docHandler, qaHandler)

	// 启动HTTP服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// 优雅关闭
	go func() {
		// 启动服务
		logger.Infof("Server is running on port %d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 优雅关闭服务器
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}

// parseFlags 解析命令行参数
func parseFlags() config {
	cfg := config{}

	// 服务配置
	flag.IntVar(&cfg.Port, "port", 8080, "Server port")
	flag.StringVar(&cfg.Mode, "mode", "debug", "Run mode (debug/release)")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (debug/info/warn/error)")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 30*time.Second, "Read timeout")
	flag.DurationVar(&cfg.WriteTimeout, "write-timeout", 30*time.Second, "Write timeout")

	// 存储配置
	flag.StringVar(&cfg.StoragePath, "storage", "./data/files", "File storage path")

	// 向量数据库配置
	flag.StringVar(&cfg.VectorDBPath, "vectordb", "./data/vectordb", "Vector database path")
	flag.IntVar(&cfg.VectorDimension, "dim", 1536, "Vector dimension")

	// 嵌入模型配置
	flag.StringVar(&cfg.EmbeddingModel, "embed-model", "text-embedding-v1", "Embedding model name")
	flag.StringVar(&cfg.EmbeddingAPIKey, "embed-key", "", "Embedding API key")

	// LLM配置
	flag.StringVar(&cfg.LLMModel, "llm-model", "qwen-turbo", "LLM model name")
	flag.StringVar(&cfg.LLMAPIKey, "llm-key", "", "LLM API key")

	// 文档处理配置
	flag.IntVar(&cfg.MaxChunkSize, "chunk-size", 1000, "Maximum text chunk size")

	// 缓存配置
	flag.StringVar(&cfg.CacheType, "cache", "memory", "Cache type (memory/redis)")

	// 数据目录配置
	flag.StringVar(&cfg.DataDir, "data-dir", "./data", "Data directory path")

	// 配置文件
	flag.StringVar(&cfg.ConfigFile, "config", "", "Path to config file")

	// 任务队列配置
	flag.BoolVar(&cfg.QueueEnabled, "queue", false, "Enable task queue")
	flag.StringVar(&cfg.QueueType, "queue-type", "redis", "Task queue type (redis)")
	flag.StringVar(&cfg.RedisAddr, "redis-addr", "localhost:6379", "Redis address for task queue")
	flag.StringVar(&cfg.RedisPassword, "redis-password", "", "Redis password")
	flag.IntVar(&cfg.RedisDB, "redis-db", 0, "Redis database number")
	flag.IntVar(&cfg.QueueConcurrency, "queue-concurrency", 10, "Task queue concurrency")
	flag.IntVar(&cfg.QueueRetryLimit, "queue-retry", 3, "Max retry attempts for failed tasks")
	flag.DurationVar(&cfg.QueueRetryDelay, "queue-retry-delay", time.Minute, "Delay between retry attempts")

	// 从环境变量获取API密钥（优先级高于命令行参数）
	if key := os.Getenv("TONGYI_API_KEY"); key != "" {
		cfg.EmbeddingAPIKey = key
		cfg.LLMAPIKey = key
	}
	if key := os.Getenv("EMBEDDING_API_KEY"); key != "" {
		cfg.EmbeddingAPIKey = key
	}
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		cfg.LLMAPIKey = key
	}
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		cfg.RedisAddr = redisAddr
	}
	if redisPassword := os.Getenv("REDIS_PASSWORD"); redisPassword != "" {
		cfg.RedisPassword = redisPassword
	}

	flag.Parse()
	return cfg
}

// updateConfigFromFile 从配置文件更新命令行参数
func updateConfigFromFile(cfg *config, appConfig *qaconfig.Config) {
	// 只更新未在命令行上明确设置的参数

	// 任务队列配置
	if flag.Lookup("queue").DefValue == fmt.Sprint(cfg.QueueEnabled) {
		cfg.QueueEnabled = appConfig.Queue.Enable
	}
	if flag.Lookup("queue-type").DefValue == cfg.QueueType {
		cfg.QueueType = appConfig.Queue.Type
	}
	if flag.Lookup("redis-addr").DefValue == cfg.RedisAddr {
		cfg.RedisAddr = appConfig.Queue.RedisAddr
	}
	if flag.Lookup("redis-password").DefValue == cfg.RedisPassword {
		cfg.RedisPassword = appConfig.Queue.RedisPassword
	}
	if flag.Lookup("redis-db").DefValue == fmt.Sprint(cfg.RedisDB) {
		cfg.RedisDB = appConfig.Queue.RedisDB
	}
	if flag.Lookup("queue-concurrency").DefValue == fmt.Sprint(cfg.QueueConcurrency) {
		cfg.QueueConcurrency = appConfig.Queue.Concurrency
	}
	if flag.Lookup("queue-retry").DefValue == fmt.Sprint(cfg.QueueRetryLimit) {
		cfg.QueueRetryLimit = appConfig.Queue.RetryLimit
	}
	if appConfig.Queue.RetryDelay > 0 {
		cfg.QueueRetryDelay = time.Duration(appConfig.Queue.RetryDelay) * time.Second
	}
}

// setupLogger 设置日志系统
func setupLogger(level string) *logrus.Logger {
	logger := middleware.GetLogger()

	// 设置日志级别
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

	return logger
}

// setupStorage 设置文件存储服务
func setupStorage(cfg config) (storage.Storage, error) {
	// 确保存储目录存在
	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %v", err)
	}

	// 创建本地存储
	return storage.NewLocalStorage(storage.LocalConfig{
		Path: cfg.StoragePath,
	})
}

// setupVectorDB 设置向量数据库
func setupVectorDB(cfg config) (vectordb.Repository, error) {
	// 确保向量数据库目录存在
	if err := os.MkdirAll(filepath.Dir(cfg.VectorDBPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create vector database directory: %v", err)
	}

	// 尝试创建FAISS向量数据库
	faissConfig := vectordb.Config{
		Type:              "faiss",
		Path:              cfg.VectorDBPath,
		Dimension:         cfg.VectorDimension,
		DistanceType:      vectordb.Cosine,
		CreateIfNotExists: true,
	}

	repo, err := vectordb.NewRepository(faissConfig)
	if err != nil {
		// 如果FAISS初始化失败，回退到内存实现
		log.Printf("Warning: Failed to initialize FAISS vector database: %v", err)
		log.Printf("Falling back to in-memory vector database")

		memoryConfig := vectordb.Config{
			Type:         "memory",
			Dimension:    cfg.VectorDimension,
			DistanceType: vectordb.Cosine,
		}
		return vectordb.NewRepository(memoryConfig)
	}

	return repo, nil
}

// setupEmbedding 设置嵌入模型客户端
func setupEmbedding(cfg config) (embedding.Client, error) {
	if cfg.EmbeddingAPIKey == "" {
		return nil, fmt.Errorf("embedding API key is required")
	}

	return embedding.NewClient("tongyi",
		embedding.WithAPIKey(cfg.EmbeddingAPIKey),
		embedding.WithModel(cfg.EmbeddingModel),
		embedding.WithDimensions(cfg.VectorDimension),
	)
}

// setupLLM 设置大语言模型客户端
func setupLLM(cfg config) (llm.Client, error) {
	if cfg.LLMAPIKey == "" {
		return nil, fmt.Errorf("LLM API key is required")
	}

	return llm.NewClient("tongyi",
		llm.WithAPIKey(cfg.LLMAPIKey),
		llm.WithModel(cfg.LLMModel),
		llm.WithMaxTokens(2048),
		llm.WithTemperature(0.7),
	)
}

// setupCache 设置缓存服务
func setupCache(cfg config) (cache.Cache, error) {
	cacheConfig := cache.Config{
		Type:            cfg.CacheType,
		DefaultTTL:      24 * time.Hour,
		CleanupInterval: 10 * time.Minute,
	}

	// 如果配置了Redis，添加Redis配置
	if cfg.CacheType == "redis" {
		cacheConfig.RedisAddr = cfg.RedisAddr
		cacheConfig.RedisPassword = cfg.RedisPassword
		// Redis数据库编号默认为0
	}

	return cache.NewCache(cacheConfig)
}

// setupDatabase 设置数据库
func setupDatabase(cfg config, logger *logrus.Logger) error {
	dbPath := filepath.Join(cfg.DataDir, "docqa.db")

	// 确保数据目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %v", err)
	}

	// 初始化数据库
	dbConfig := &database.Config{
		Type: "sqlite",
		DSN:  dbPath,
	}

	return database.Setup(dbConfig, logger)
}

// setupTaskQueue 设置任务队列
func setupTaskQueue(cfg config, logger *logrus.Logger) (taskqueue.Queue, error) {
	if !cfg.QueueEnabled {
		return nil, nil
	}

	// 根据配置创建任务队列
	queueConfig := &taskqueue.Config{
		RedisAddr:     cfg.RedisAddr,
		RedisPassword: cfg.RedisPassword,
		RedisDB:       cfg.RedisDB,
		Concurrency:   cfg.QueueConcurrency,
		RetryLimit:    cfg.QueueRetryLimit,
		RetryDelay:    cfg.QueueRetryDelay,
	}

	logger.WithFields(logrus.Fields{
		"type":        cfg.QueueType,
		"redis_addr":  cfg.RedisAddr,
		"concurrency": cfg.QueueConcurrency,
		"retry_limit": cfg.QueueRetryLimit,
	}).Info("Setting up task queue")

	queue, err := taskqueue.NewQueue(cfg.QueueType, queueConfig)
	if err != nil {
		return nil, err
	}

	return queue, nil
}
