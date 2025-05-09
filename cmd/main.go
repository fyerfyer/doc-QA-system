package main

import (
	"context"
	"errors"
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
	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/fyerfyer/doc-QA-system/config"
	"github.com/fyerfyer/doc-QA-system/internal/adapters"
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/database"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/taskqueue"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// 命令行参数结构
type cmdArgs struct {
	ConfigPath     string // 配置文件路径
	Port           int    // 服务端口（覆盖配置文件）
	LogLevel       string // 日志级别（覆盖配置文件）
	EmbeddingKey   string // 嵌入API密钥（覆盖配置文件）
	LLMKey         string // LLM API密钥（覆盖配置文件）
	EnablePython   bool   // 是否启用Python服务（覆盖配置文件）
	EnableTaskQ    bool   // 是否启用任务队列（覆盖配置文件）
	ListenerEnable bool   // 是否启用状态更新监听器
}

func main() {
	// 解析命令行参数
	args := parseCmdArgs()

	// 加载配置
	cfg, err := loadConfig(args)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 覆盖配置文件中的值（如果命令行参数有指定）
	applyOverrides(cfg, args)

	// 设置Gin模式
	if cfg.Server.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 初始化日志
	logger := setupLogger(cfg.Server.LogLevel)
	logger.Info("正在启动文档问答系统...")

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

	// 初始化RAG服务
	ragService := llm.NewRAG(llmClient,
		llm.WithRAGMaxTokens(cfg.LLM.MaxTokens),
		llm.WithRAGTemperature(float32(cfg.LLM.Temperature)),
	)

	// 初始化QA服务
	qaService := services.NewQAService(
		embeddingClient,
		vectorDB,
		llmClient,
		ragService,
		cacheService,
		services.WithMinScore(float32(cfg.VectorDB.MinScore)),
		services.WithSearchLimit(cfg.VectorDB.MaxResults),
	)

	// 如果启用任务队列，则创建或获取任务队列及状态监听器
	var docService adapters.DocumentProcessingService
	var statusListener *taskqueue.UpdateListener

	if cfg.TaskQueue.Enable {
		// 创建文档服务（使用适配器工厂选择适当的实现）
		var err error
		docService, err = adapters.CreateDocumentService(cfg, fileStorage, logger)
		if err != nil {
			logger.Fatalf("Failed to create document service: %v", err)
		}

		// 如果启用了Python服务，创建状态监听器
		if cfg.TaskQueue.PythonTasks.DocumentProcess {
			// 创建Redis客户端用于监听
			redisOpt := &redis.Options{
				Addr:     cfg.TaskQueue.Address,
				Password: cfg.TaskQueue.Password,
				DB:       cfg.TaskQueue.DB,
			}
			redisClient := redis.NewClient(redisOpt)

			// 测试Redis连接
			if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
				logger.Warnf("Failed to connect to Redis for status listener: %v", err)
			} else {
				// 创建状态更新监听器
				statusListener = taskqueue.NewUpdateListener(redisClient)

				// 设置文档更新管理器
				statusManager := docService.GetStatusManager()
				if statusManager != nil {
					// 实现UpdateListener接口，添加文档状态更新器
					statusListener.SetDocumentUpdater(statusManager)

					// 如果启用了监听器，启动它
					if args.ListenerEnable {
						statusListener.Start()
						logger.Info("文档状态更新监听器已启动")
						defer statusListener.Stop()
					}
				}
			}
		}
	} else {
		// 创建标准文档服务
		parser, _ := document.ParserFactory("dummy.txt") // 创建默认解析器
		splitter := document.NewTextSplitter(document.SplitterConfig{
			ChunkSize:    cfg.Embed.ChunkSize,
			ChunkOverlap: cfg.Embed.ChunkOverlap,
		})

		// 转换为DocumentProcessingService接口
		stdDocService := services.NewDocumentService(
			fileStorage,
			parser,
			splitter,
			embeddingClient,
			vectorDB,
			services.WithLogger(logger),
			services.WithBatchSize(cfg.Embed.BatchSize),
		)

		docService = stdDocService
	}

	// 初始化API处理器
	docHandler := handler.NewDocumentHandler(docService, fileStorage)
	qaHandler := handler.NewQAHandler(qaService)

	// 设置路由
	r := api.SetupRouter(docHandler, qaHandler)

	// 启动HTTP服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// 优雅关闭
	go func() {
		// 启动服务
		logger.Infof("Server is running on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待终止信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("正在关闭服务器...")

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 优雅关闭服务器
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("服务器已退出")
}

// parseCmdArgs 解析命令行参数
func parseCmdArgs() cmdArgs {
	args := cmdArgs{}

	// 配置文件路径
	flag.StringVar(&args.ConfigPath, "config", "", "Path to configuration file")

	// 服务器配置
	flag.IntVar(&args.Port, "port", 0, "Server port (overrides config)")
	flag.StringVar(&args.LogLevel, "log-level", "", "Log level: debug/info/warn/error (overrides config)")

	// API密钥
	flag.StringVar(&args.EmbeddingKey, "embed-key", "", "Embedding API key (overrides config)")
	flag.StringVar(&args.LLMKey, "llm-key", "", "LLM API key (overrides config)")

	// 任务队列和Python服务
	flag.BoolVar(&args.EnableTaskQ, "task-queue", false, "Enable task queue (overrides config)")
	flag.BoolVar(&args.EnablePython, "python", false, "Enable Python services (overrides config)")
	flag.BoolVar(&args.ListenerEnable, "listener", true, "Enable document status update listener")

	flag.Parse()
	return args
}

// loadConfig 加载配置文件
func loadConfig(args cmdArgs) (*config.Config, error) {
	return config.Load(args.ConfigPath)
}

// applyOverrides 应用命令行参数覆盖配置
func applyOverrides(cfg *config.Config, args cmdArgs) {
	// 端口
	if args.Port > 0 {
		cfg.Server.Port = args.Port
	}

	// 日志级别
	if args.LogLevel != "" {
		cfg.Server.LogLevel = args.LogLevel
	}

	// API密钥
	if args.EmbeddingKey != "" {
		cfg.Embed.APIKey = args.EmbeddingKey
	}
	if args.LLMKey != "" {
		cfg.LLM.APIKey = args.LLMKey
	}

	// 任务队列和Python服务
	if args.EnableTaskQ {
		cfg.TaskQueue.Enable = true
	}
	if args.EnablePython {
		cfg.TaskQueue.PythonTasks.DocumentProcess = true
	}

	// 从环境变量获取API密钥（最高优先级）
	if key := os.Getenv("TONGYI_API_KEY"); key != "" {
		cfg.Embed.APIKey = key
		cfg.LLM.APIKey = key
	}
	if key := os.Getenv("EMBEDDING_API_KEY"); key != "" {
		cfg.Embed.APIKey = key
	}
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		cfg.LLM.APIKey = key
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
func setupStorage(cfg *config.Config) (storage.Storage, error) {
	// 根据配置选择存储类型
	if cfg.Storage.Type == "local" {
		// 确保存储目录存在
		if err := os.MkdirAll(cfg.Storage.Path, 0755); err != nil {
			return nil, fmt.Errorf("failed to create storage directory: %v", err)
		}

		// 创建本地存储
		return storage.NewLocalStorage(storage.LocalConfig{
			Path: cfg.Storage.Path,
		})
	} else if cfg.Storage.Type == "minio" {
		// 创建MinIO存储
		return storage.NewMinioStorage(storage.MinioConfig{
			Endpoint:  cfg.Storage.Path,
			Bucket:    cfg.Storage.Bucket,
			AccessKey: cfg.Storage.AccessKey,
			SecretKey: cfg.Storage.SecretKey,
			UseSSL:    cfg.Storage.UseSSL,
		})
	}

	return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
}

// setupVectorDB 设置向量数据库
func setupVectorDB(cfg *config.Config) (vectordb.Repository, error) {
	// 确保向量数据库目录存在
	if cfg.VectorDB.Type == "faiss" {
		if err := os.MkdirAll(filepath.Dir(cfg.VectorDB.Path), 0755); err != nil {
			return nil, fmt.Errorf("failed to create vector database directory: %v", err)
		}
	}

	// 创建向量数据库
	vdbConfig := vectordb.Config{
		Type:              cfg.VectorDB.Type,
		Path:              cfg.VectorDB.Path,
		Dimension:         cfg.VectorDB.Dim,
		DistanceType:      vectordb.DistanceType(cfg.VectorDB.Distance),
		CreateIfNotExists: true,
	}

	repo, err := vectordb.NewRepository(vdbConfig)
	if err != nil {
		// 如果配置的向量数据库初始化失败，回退到内存实现
		log.Printf("Warning: Failed to initialize %s vector database: %v", cfg.VectorDB.Type, err)
		log.Printf("Falling back to in-memory vector database")

		memoryConfig := vectordb.Config{
			Type:         "memory",
			Dimension:    cfg.VectorDB.Dim,
			DistanceType: vectordb.DistanceType(cfg.VectorDB.Distance),
		}
		return vectordb.NewRepository(memoryConfig)
	}

	return repo, nil
}

// setupEmbedding 设置嵌入模型客户端
func setupEmbedding(cfg *config.Config) (embedding.Client, error) {
	if cfg.Embed.APIKey == "" {
		return nil, fmt.Errorf("embedding API key is required")
	}

	options := []embedding.Option{
		embedding.WithAPIKey(cfg.Embed.APIKey),
		embedding.WithModel(cfg.Embed.Model),
		embedding.WithDimensions(cfg.VectorDB.Dim),
	}

	// 如果配置了自定义端点，添加它
	if cfg.Embed.Endpoint != "" {
		options = append(options, embedding.WithBaseURL(cfg.Embed.Endpoint))
	}

	return embedding.NewClient(cfg.Embed.Provider, options...)
}

// setupLLM 设置大语言模型客户端
func setupLLM(cfg *config.Config) (llm.Client, error) {
	if cfg.LLM.APIKey == "" {
		return nil, fmt.Errorf("LLM API key is required")
	}

	options := []llm.Option{
		llm.WithAPIKey(cfg.LLM.APIKey),
		llm.WithModel(cfg.LLM.Model),
		llm.WithMaxTokens(cfg.LLM.MaxTokens),
		llm.WithTemperature(float32(cfg.LLM.Temperature)),
	}

	// 如果配置了自定义端点，添加它
	if cfg.LLM.Endpoint != "" {
		options = append(options, llm.WithBaseURL(cfg.LLM.Endpoint))
	}

	return llm.NewClient(cfg.LLM.Provider, options...)
}

// setupCache 设置缓存服务
func setupCache(cfg *config.Config) (cache.Cache, error) {
	if !cfg.Cache.Enable {
		// 如果禁用缓存，返回无操作缓存实现
		return cache.NewNoopCache(), nil
	}

	cacheConfig := cache.Config{
		Type:            cfg.Cache.Type,
		DefaultTTL:      time.Duration(cfg.Cache.TTL) * time.Second,
		CleanupInterval: 10 * time.Minute,
	}

	// 如果配置了Redis缓存，添加Redis配置
	if cfg.Cache.Type == "redis" {
		cacheConfig.RedisAddr = cfg.Cache.Address
		cacheConfig.RedisPassword = cfg.Cache.Password
		cacheConfig.RedisDB = cfg.Cache.DB
	}

	return cache.NewCache(cacheConfig)
}

// setupDatabase 设置数据库
func setupDatabase(cfg *config.Config, logger *logrus.Logger) error {
	// 确保数据目录存在
	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %v", err)
	}

	// 初始化数据库
	dbConfig := &database.Config{
		Type: cfg.Database.Type,
		DSN:  cfg.Database.Path,
	}

	return database.Setup(dbConfig, logger)
}
