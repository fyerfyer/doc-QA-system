package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestMockClientGenerate 测试使用Mock客户端的文本生成
func TestMockClientGenerate(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 准备预期的响应
	expectedResp := &Response{
		Text:       "这是生成的测试文本",
		TokenCount: 5,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 设置Mock期望
	mockClient.EXPECT().Generate(mock.Anything, "测试提示词", mock.Anything).Return(expectedResp, nil)

	// 调用方法
	ctx := context.Background()
	resp, err := mockClient.Generate(ctx, "测试提示词")

	// 验证结果
	assert.NoError(t, err)
	assert.Equal(t, expectedResp.Text, resp.Text)
	assert.Equal(t, expectedResp.TokenCount, resp.TokenCount)
}

// TestMockClientChat 测试使用Mock客户端的对话功能
func TestMockClientChat(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 准备消息
	messages := []Message{
		{Role: RoleUser, Content: "你好"},
		{Role: RoleAssistant, Content: "您好！有什么可以帮助您的？"},
		{Role: RoleUser, Content: "今天天气怎么样？"},
	}

	// 准备预期的响应
	expectedResp := &Response{
		Text: "今天天气晴朗，温度适宜。",
		Messages: []Message{
			{Role: RoleAssistant, Content: "今天天气晴朗，温度适宜。"},
		},
		TokenCount: 10,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 设置Mock期望
	mockClient.EXPECT().Chat(mock.Anything, messages, mock.Anything).Return(expectedResp, nil)

	// 调用方法
	ctx := context.Background()
	resp, err := mockClient.Chat(ctx, messages)

	// 验证结果
	assert.NoError(t, err)
	assert.Equal(t, expectedResp.Text, resp.Text)
	assert.Len(t, resp.Messages, 1)
	assert.Equal(t, expectedResp.Messages[0].Content, resp.Messages[0].Content)
}

// TestMockClientErrors 测试错误处理
func TestMockClientErrors(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 设置错误期望 - 空提示词
	emptyPromptErr := NewLLMError(ErrCodeEmptyPrompt, ErrMsgEmptyPrompt)
	mockClient.EXPECT().Generate(mock.Anything, "", mock.Anything).Return(nil, emptyPromptErr)

	// 调用Generate方法
	ctx := context.Background()
	_, err := mockClient.Generate(ctx, "")

	// 验证错误
	assert.Error(t, err)
	var llmErr LLMError
	assert.ErrorAs(t, err, &llmErr)
	assert.Equal(t, ErrCodeEmptyPrompt, llmErr.Code)

	// 测试无效API密钥错误
	apiKeyErr := NewLLMError(ErrCodeInvalidAPIKey, ErrMsgInvalidAPIKey)
	mockClient.EXPECT().Chat(mock.Anything, mock.Anything, mock.Anything).Return(nil, apiKeyErr)

	// 调用Chat方法
	_, err = mockClient.Chat(ctx, []Message{{Role: RoleUser, Content: "测试"}})
	assert.Error(t, err)
	assert.ErrorAs(t, err, &llmErr)
	assert.Equal(t, ErrCodeInvalidAPIKey, llmErr.Code)
}

// TestMockClientName 测试模型名称方法
func TestMockClientName(t *testing.T) {
	mockClient := NewMockClient(t)
	mockClient.EXPECT().Name().Return("mock-model")

	name := mockClient.Name()
	assert.Equal(t, "mock-model", name)
}

// TestTongyiClientIntegration 测试通义千问客户端集成
// 只有在设置TONGYI_API_KEY环境变量时才运行
func TestTongyiClientIntegration(t *testing.T) {
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("Haven't set TONGYI_API_KEY environment variable, skipping test")
	}

	// 使用最短超时创建客户端，节省资源
	client, err := NewTongyiClient(
		WithAPIKey(apiKey),
		WithModel(ModelQwenTurbo), // 使用速度最快的模型
		WithTimeout(10*time.Second),
	)
	require.NoError(t, err, "创建客户端失败")

	// 使用非常短的提示词，减少token使用
	t.Run("generate test", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		resp, err := client.Generate(ctx, "你好", WithGenerateMaxTokens(5))
		if err != nil {
			t.Logf("API calling error: %v", err)
			t.Skip("Skipping API test")
			return
		}

		// 基本验证
		assert.NotEmpty(t, resp.Text, "Response text should not be empty")
		assert.NotZero(t, resp.TokenCount, "Token count should be greater than 0")
		assert.Equal(t, ModelQwenTurbo, resp.ModelName, "Model name should match")
	})

	// 测试Chat方法
	t.Run("chat test", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		messages := []Message{
			{Role: RoleUser, Content: "简单问候"},
		}

		resp, err := client.Chat(ctx, messages, WithChatMaxTokens(5))
		if err != nil {
			t.Logf("API calling error: %v", err)
			t.Skip("Skipping API test")
			return
		}

		// 基本验证
		assert.NotEmpty(t, resp.Text, "Response text should not be empty")
		assert.NotZero(t, resp.TokenCount, "Token count should be greater than 0")
	})
}

// TestConfigAndOptions 测试配置选项
func TestConfigAndOptions(t *testing.T) {
	// 测试默认配置
	cfg := DefaultConfig()
	assert.Equal(t, ModelQwenTurbo, cfg.Model)
	assert.Equal(t, 60*time.Second, cfg.Timeout)

	// 测试应用选项
	cfg = NewConfig(
		WithAPIKey("test-key"),
		WithModel("custom-model"),
		WithTimeout(30*time.Second),
		WithMaxRetries(5),
		WithMaxTokens(100),
		WithTemperature(0.5),
		WithTopP(0.8),
	)

	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "custom-model", cfg.Model)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 100, cfg.MaxTokens)
	assert.Equal(t, float32(0.5), cfg.Temperature)
	assert.Equal(t, float32(0.8), cfg.TopP)
}

// TestGenerateOptions 测试生成选项
func TestGenerateOptions(t *testing.T) {
	opts := &GenerateOptions{}

	// 应用选项
	maxTokens := 123
	WithGenerateMaxTokens(maxTokens)(opts)
	assert.Equal(t, &maxTokens, opts.MaxTokens)

	temp := float32(0.75)
	WithGenerateTemperature(temp)(opts)
	assert.Equal(t, &temp, opts.Temperature)

	topP := float32(0.9)
	WithGenerateTopP(topP)(opts)
	assert.Equal(t, &topP, opts.TopP)

	topK := 40
	WithGenerateTopK(topK)(opts)
	assert.Equal(t, &topK, opts.TopK)

	WithGenerateStream(true)(opts)
	assert.True(t, opts.Stream)
}

// TestChatOptions 测试聊天选项
func TestChatOptions(t *testing.T) {
	opts := &ChatOptions{}

	// 应用选项
	maxTokens := 123
	WithChatMaxTokens(maxTokens)(opts)
	assert.Equal(t, &maxTokens, opts.MaxTokens)

	temp := float32(0.75)
	WithChatTemperature(temp)(opts)
	assert.Equal(t, &temp, opts.Temperature)

	topP := float32(0.9)
	WithChatTopP(topP)(opts)
	assert.Equal(t, &topP, opts.TopP)

	topK := 40
	WithChatTopK(topK)(opts)
	assert.Equal(t, &topK, opts.TopK)

	WithChatStream(true)(opts)
	assert.True(t, opts.Stream)
}

// TestClientFactory 测试客户端工厂功能
func TestClientFactory(t *testing.T) {
	// 注册测试工厂
	testFactory := func(opts ...Option) (Client, error) {
		return NewMockClient(t), nil
	}
	RegisterClient("test-factory", testFactory)

	// 使用工厂创建客户端
	client, err := NewClient("test-factory")
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// 测试无效的客户端类型
	_, err = NewClient("invalid-type")
	assert.Error(t, err)
	var llmErr LLMError
	assert.ErrorAs(t, err, &llmErr)
	assert.Equal(t, ErrCodeInvalidRequest, llmErr.Code)
}
