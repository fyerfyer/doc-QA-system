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
	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/document"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/services"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
	"github.com/fyerfyer/doc-QA-system/pkg/storage"
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
}

func main() {
	// 解析命令行参数
	cfg := parseFlags()

	// 设置Gin模式
	gin.SetMode(cfg.Mode)

	// 初始化日志
	logger := setupLogger(cfg.LogLevel)
	logger.Info("Starting Document QA System...")

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
	documentService := services.NewDocumentService(
		fileStorage,
		nil, // document.Parser通过调用ParserFactory动态创建
		splitter,
		embeddingClient,
		vectorDB,
		services.WithBatchSize(16),
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

	flag.Parse()
	return cfg
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
		cacheConfig.RedisAddr = os.Getenv("REDIS_ADDR")
		if cacheConfig.RedisAddr == "" {
			cacheConfig.RedisAddr = "localhost:6379"
		}
		cacheConfig.RedisPassword = os.Getenv("REDIS_PASSWORD")
		// Redis数据库编号默认为0
	}

	return cache.NewCache(cacheConfig)
}
