package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用程序配置结构体
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Storage       StorageConfig       `mapstructure:"storage"`
	VectorDB      VectorDBConfig      `mapstructure:"vectordb"`
	LLM           LLMConfig           `mapstructure:"llm"`
	Embed         EmbedConfig         `mapstructure:"embed"`
	Cache         CacheConfig         `mapstructure:"cache"`
	Queue         QueueConfig         `mapstructure:"queue"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Document      DocumentConfig      `mapstructure:"document"`
	Search        SearchConfig        `mapstructure:"search"`
	PythonService PythonServiceConfig `mapstructure:"python_service"` // 新增Python服务配置
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host string `mapstructure:"host"` // 服务器主机
	Port int    `mapstructure:"port"` // 服务器端口
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type      string `mapstructure:"type"`     // 存储类型：local 或 minio
	Path      string `mapstructure:"path"`     // 本地存储路径
	Bucket    string `mapstructure:"bucket"`   // MinIO桶名称
	Endpoint  string `mapstructure:"endpoint"` // MinIO端点
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	UseSSL    bool   `mapstructure:"use_ssl"` // 是否使用SSL
}

// VectorDBConfig 向量数据库配置
type VectorDBConfig struct {
	Type     string `mapstructure:"type"`     // 向量数据库类型：faiss 或 qdrant
	Path     string `mapstructure:"path"`     // 数据库文件路径或服务器地址
	Dim      int    `mapstructure:"dim"`      // 向量维度
	Distance string `mapstructure:"distance"` // 距离度量方式：cosine, l2, dot
}

// LLMConfig 大语言模型配置
type LLMConfig struct {
	Provider    string  `mapstructure:"provider"`    // 提供商：openai, ollama, etc
	Model       string  `mapstructure:"model"`       // 模型名称
	APIKey      string  `mapstructure:"api_key"`     // API密钥
	Endpoint    string  `mapstructure:"endpoint"`    // API端点
	MaxTokens   int     `mapstructure:"max_tokens"`  // 最大生成token数量
	Temperature float32 `mapstructure:"temperature"` // 采样温度
}

// EmbedConfig 向量嵌入模型配置
type EmbedConfig struct {
	Provider   string `mapstructure:"provider"`   // 提供商：openai, local, etc
	Model      string `mapstructure:"model"`      // 模型名称
	APIKey     string `mapstructure:"api_key"`    // API密钥（如果需要）
	Endpoint   string `mapstructure:"endpoint"`   // API端点
	BatchSize  int    `mapstructure:"batch_size"` // 批处理大小
	Dimensions int    `mapstructure:"dimensions"` // 向量维度
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Enable   bool   `mapstructure:"enable"`   // 是否启用缓存
	Type     string `mapstructure:"type"`     // 缓存类型：memory 或 redis
	Address  string `mapstructure:"address"`  // Redis地址
	Password string `mapstructure:"password"` // Redis密码
	DB       int    `mapstructure:"db"`       // Redis数据库
	TTL      int    `mapstructure:"ttl"`      // 缓存TTL（秒）
}

// QueueConfig 任务队列配置
type QueueConfig struct {
	Enable        bool   `mapstructure:"enable"`         // 是否启用任务队列
	Type          string `mapstructure:"type"`           // 队列类型：redis或memory
	RedisAddr     string `mapstructure:"redis_addr"`     // Redis地址
	RedisPassword string `mapstructure:"redis_password"` // Redis密码
	RedisDB       int    `mapstructure:"redis_db"`       // Redis数据库编号
	Concurrency   int    `mapstructure:"concurrency"`    // 任务处理并发数
	RetryLimit    int    `mapstructure:"retry_limit"`    // 任务最大重试次数
	RetryDelay    int    `mapstructure:"retry_delay"`    // 重试延迟(秒)
	CallbackURL   string `mapstructure:"callback_url"`   // 回调URL
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type string `mapstructure:"type"` // 数据库类型: sqlite, mysql, postgres
	DSN  string `mapstructure:"dsn"`  // 数据源名称
}

// DocumentConfig 文档处理配置
type DocumentConfig struct {
	ChunkSize    int `mapstructure:"chunk_size"`    // 分块大小
	ChunkOverlap int `mapstructure:"chunk_overlap"` // 分块重叠大小
}

// SearchConfig 搜索配置
type SearchConfig struct {
	Limit    int     `mapstructure:"limit"`     // 搜索结果数量限制
	MinScore float32 `mapstructure:"min_score"` // 最低相似度分数
}

// PythonServiceConfig Python服务配置
type PythonServiceConfig struct {
	BaseURL       string        `mapstructure:"base_url"`       // Python服务基础URL
	Timeout       time.Duration `mapstructure:"timeout"`        // 请求超时时间
	MaxRetries    int           `mapstructure:"max_retries"`    // 最大重试次数
	RetryDelay    time.Duration `mapstructure:"retry_delay"`    // 重试间隔
	EnableTLS     bool          `mapstructure:"enable_tls"`     // 是否启用TLS
	AllowInsecure bool          `mapstructure:"allow_insecure"` // 允许不安全的TLS连接
}

// Load 从文件和环境变量加载配置
func Load(configPath string) (*Config, error) {
	var config Config

	// 设置默认配置路径
	if configPath == "" {
		configPath = "config.yaml" // 默认在当前目录寻找config.yaml
	}

	// 初始化viper
	v := viper.New()

	// 设置配置文件路径和类型
	v.SetConfigFile(configPath)

	// 尝试读取配置文件
	if err := v.ReadInConfig(); err != nil {
		// 如果找不到配置文件，创建一个默认配置文件
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Warning: Config file not found at %s, using defaults", configPath)
			setDefaults(v)
			// 创建默认配置文件
			dir := filepath.Dir(configPath)
			if err := os.MkdirAll(dir, 0755); err == nil {
				if err := v.WriteConfigAs(configPath); err != nil {
					log.Printf("Warning: Could not write default config to %s: %v", configPath, err)
				}
			}
		} else {
			return nil, fmt.Errorf("failed to read config file: %v", err)
		}
	} else {
		log.Printf("Using config file: %s", v.ConfigFileUsed())
	}

	// 设置默认值
	setDefaults(v)

	// 支持环境变量覆盖
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 解析配置到结构体
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	// 添加新函数调用来处理环境变量替换
	resConfig := processEnvironmentVariables(&config)

	return resConfig, nil
}

// 添加这个新函数来处理所有配置项中的环境变量
func processEnvironmentVariables(cfg *Config) *Config {
	// 处理嵌入API密钥
	if strings.HasPrefix(cfg.Embed.APIKey, "${") && strings.HasSuffix(cfg.Embed.APIKey, "}") {
		envVar := cfg.Embed.APIKey[2 : len(cfg.Embed.APIKey)-1]
		if envVal := os.Getenv(envVar); envVal != "" {
			cfg.Embed.APIKey = envVal
		}
	}

	// 处理LLM API密钥
	if strings.HasPrefix(cfg.LLM.APIKey, "${") && strings.HasSuffix(cfg.LLM.APIKey, "}") {
		envVar := cfg.LLM.APIKey[2 : len(cfg.LLM.APIKey)-1]
		if envVal := os.Getenv(envVar); envVal != "" {
			cfg.LLM.APIKey = envVal
		}
	}

	// 可以添加更多配置项的处理

	return cfg
}

// setDefaults 设置配置的默认值
func setDefaults(v *viper.Viper) {
	// 服务器默认配置
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)

	// 存储默认配置
	v.SetDefault("storage.type", "local")
	v.SetDefault("storage.path", "./uploads")
	v.SetDefault("storage.bucket", "docqa")
	v.SetDefault("storage.use_ssl", false)

	// 向量数据库默认配置
	v.SetDefault("vectordb.type", "faiss")
	v.SetDefault("vectordb.path", "./vectordb")
	v.SetDefault("vectordb.dim", 1024) // Qwen embedding 维度
	v.SetDefault("vectordb.distance", "cosine")

	// LLM默认配置
	v.SetDefault("llm.provider", "openai")
	v.SetDefault("llm.model", "gpt-3.5-turbo")
	v.SetDefault("llm.endpoint", "https://api.openai.com/v1")
	v.SetDefault("llm.max_tokens", 1000)

	// Embedding默认配置
	v.SetDefault("embed.provider", "openai")
	v.SetDefault("embed.model", "text-embedding-3-small")
	v.SetDefault("embed.endpoint", "https://api.openai.com/v1")
	v.SetDefault("embed.batch_size", 10)

	// 缓存默认配置
	v.SetDefault("cache.enable", true)
	v.SetDefault("cache.type", "memory")
	v.SetDefault("cache.ttl", 3600) // 1小时

	// 队列默认配置
	v.SetDefault("queue.enable", false)
	v.SetDefault("queue.type", "redis")
	v.SetDefault("queue.redis_addr", "localhost:6379")
	v.SetDefault("queue.redis_db", 0)
	v.SetDefault("queue.concurrency", 10)
	v.SetDefault("queue.retry_limit", 3)
	v.SetDefault("queue.retry_delay", 60) // 60秒

	// 数据库默认配置
	v.SetDefault("database.type", "sqlite")
	v.SetDefault("database.dsn", "data/docqa.db")

	// 文档处理默认配置
	v.SetDefault("document.chunk_size", 1000)
	v.SetDefault("document.chunk_overlap", 200)

	// 搜索默认配置
	v.SetDefault("search.limit", 10)
	v.SetDefault("search.min_score", 0.5)

	// Python服务默认配置
	v.SetDefault("python_service.base_url", "http://localhost:8000/api/python")
	v.SetDefault("python_service.timeout", "30s")
	v.SetDefault("python_service.max_retries", 3)
	v.SetDefault("python_service.retry_delay", "1s")
	v.SetDefault("python_service.enable_tls", false)
	v.SetDefault("python_service.allow_insecure", false)
}
