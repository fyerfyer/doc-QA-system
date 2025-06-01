package pyprovider

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// 测试环境变量名称
const defaultTestServiceURL   = "http://localhost:8000/api" // 默认测试URL

// 获取测试配置
func getTestConfig() *PyServiceConfig {
    return &PyServiceConfig{
        BaseURL:    defaultTestServiceURL,
        Timeout:    10 * time.Second,
        MaxRetries: 2,
        RetryDelay: 500 * time.Millisecond,
    }
}

// 创建测试用LLM客户端
func getTestLLMClient(t *testing.T) *LLMClient {
    config := getTestConfig()
    client, err := NewClient(config)
    require.NoError(t, err, "failed to create LLM client")

    return NewLLMClient(client)
}

// TestGenerateText 测试文本生成功能
func TestGenerateText(t *testing.T) {
    client := getTestLLMClient(t)
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    t.Run("generate simple text", func(t *testing.T) {
        prompt := "你好，请用一句话介绍一下自己"
        response, err := client.Generate(ctx, prompt)

        assert.NoError(t, err, "generated text should not return an error")
        assert.NotEmpty(t, response.Text, "generated text should not be empty")
        assert.NotEmpty(t, response.Model, "model name should not be empty")
        assert.Greater(t, response.TotalTokens, 0, "total tokens should be greater than 0")
    })

    t.Run("generate with options", func(t *testing.T) {
        prompt := "列出3个常见的编程语言"
        response, err := client.Generate(ctx, prompt, 
            WithModel("default"),
            WithTemperature(0.5),
            WithMaxTokens(100))

        assert.NoError(t, err, "using options should not return an error")
        assert.NotEmpty(t, response.Text, "generated text should not be empty")
    })
}

// TestChatCompletion 测试聊天对话功能
func TestChatCompletion(t *testing.T) {
    client := getTestLLMClient(t)
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    t.Run("simple chat", func(t *testing.T) {
        messages := []Message{
            {Role: RoleSystem, Content: "你是一个有用的AI助手"},
            {Role: RoleUser, Content: "你好，请用一句话告诉我今天是星期几？"},
        }

        response, err := client.Chat(ctx, messages)

        assert.NoError(t, err, "chat completion should not return an error")
        assert.NotEmpty(t, response.Text, "chat response text should not be empty")
        assert.NotEmpty(t, response.Model, "model name should not be empty")
    })

    t.Run("multi-turn chat", func(t *testing.T) {
        messages := []Message{
            {Role: RoleSystem, Content: "你是一个有用的AI助手"},
            {Role: RoleUser, Content: "你好"},
            {Role: RoleAssistant, Content: "您好！有什么我可以帮助您的吗？"},
            {Role: RoleUser, Content: "解释一下什么是向量数据库"},
        }

        response, err := client.Chat(ctx, messages, 
            WithChatModel("default"),
            WithChatTemperature(0.7))

        assert.NoError(t, err, "multi-turn chat should not return an error")
        assert.NotEmpty(t, response.Text, "chat response text should not be empty")
        assert.Contains(t, response.Text, "向量", "chat response should contain '向量'")
    })
}

// TestRAGAnswer 测试RAG回答功能
func TestRAGAnswer(t *testing.T) {
    client := getTestLLMClient(t)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    t.Run("simple question", func(t *testing.T) {
        query := "什么是向量数据库？"

        response, err := client.Answer(ctx, query)

        require.NoError(t, err, "RAG search should not return an error")

        assert.NotEmpty(t, response.Text, "RAG response text should not be empty")
    })

    t.Run("with options", func(t *testing.T) {
        query := "解释一下Go语言的优点"

        response, err := client.Answer(ctx, query,
            WithRAGModel("default"),
            WithTopK(3),
            WithEnableCitation(true))

        require.NoError(t, err, "RAG search with options should not return an error")

        assert.NotEmpty(t, response.Text, "RAG response text should not be empty")
    })
}