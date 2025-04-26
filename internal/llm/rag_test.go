package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestRAGBasicFunctionality 测试RAG的基本功能
func TestRAGBasicFunctionality(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 测试问题和上下文
	question := "什么是向量数据库？"
	contexts := []string{
		"向量数据库是一种专门存储和检索向量数据的数据库系统。",
		"与传统数据库不同，向量数据库针对高维向量的相似度搜索进行了优化。",
	}

	// 设置模拟响应
	expectedAnswer := "向量数据库是一种专门存储和检索向量数据的数据库系统，与传统数据库不同，它针对高维向量的相似度搜索进行了优化。"
	mockResponse := &Response{
		Text:       expectedAnswer,
		TokenCount: 50,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 期望模型根据问题和上下文生成回答
	mockClient.EXPECT().
		Generate(mock.Anything, mock.MatchedBy(func(prompt string) bool {
			// 验证提示词中包含问题和上下文
			return strings.Contains(prompt, question) &&
				strings.Contains(prompt, "向量数据库是一种专门") &&
				strings.Contains(prompt, "与传统数据库不同")
		}), mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	// 创建RAG服务
	rag := NewRAG(mockClient)

	// 调用RAG服务生成回答
	ctx := context.Background()
	response, err := rag.Answer(ctx, question, contexts)

	// 验证结果
	require.NoError(t, err)
	assert.Equal(t, expectedAnswer, response.Answer)
	assert.Len(t, response.Sources, 2)
	assert.Equal(t, contexts[0], response.Sources[0].Content)
}

// TestRAGWithDifferentTemplates 测试使用不同的提示词模板
func TestRAGWithDifferentTemplates(t *testing.T) {
	// 测试问题和上下文
	question := "什么是RAG？"
	contexts := []string{"RAG是检索增强生成的缩写，是一种结合文档检索和语言模型的技术。"}

	// 设置模拟响应
	mockResponse := &Response{
		Text:       "RAG是检索增强生成的缩写，一种结合文档检索和语言模型的技术。",
		TokenCount: 30,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 默认模板测试
	t.Run("default template", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockClient.EXPECT().
			Generate(mock.Anything, mock.MatchedBy(func(prompt string) bool {
				return strings.Contains(prompt, "基于下面提供的参考上下文回答问题")
			}), mock.Anything, mock.Anything).
			Return(mockResponse, nil)

		defaultRag := NewRAG(mockClient)
		ctx := context.Background()
		defaultResp, err := defaultRag.Answer(ctx, question, contexts)
		require.NoError(t, err)
		assert.Equal(t, mockResponse.Text, defaultResp.Answer)
	})

	// 深度思考模板测试
	t.Run("deep thinking template", func(t *testing.T) {
		mockClient := NewMockClient(t)
		mockClient.EXPECT().
			Generate(mock.Anything, mock.MatchedBy(func(prompt string) bool {
				return strings.Contains(prompt, "思考过程")
			}), mock.Anything, mock.Anything).
			Return(mockResponse, nil)

		deepThinkingRag := NewRAG(mockClient, WithDeepThinking())
		ctx := context.Background()
		deepResp, err := deepThinkingRag.Answer(ctx, question, contexts)
		require.NoError(t, err)
		assert.Equal(t, mockResponse.Text, deepResp.Answer)
	})
}

// TestRAGWithCustomTemplate 测试自定义提示词模板
func TestRAGWithCustomTemplate(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 自定义模板
	customTemplate := `请根据以下信息回答问题:
信息源:
{{.Context}}

问题:
{{.Question}}

答案:`

	// 测试问题和上下文
	question := "Go语言的优势是什么？"
	contexts := []string{"Go语言具有简洁的语法、强大的并发支持和快速的编译速度。"}

	// 模拟响应
	mockResponse := &Response{
		Text:       "Go语言的优势是简洁的语法、强大的并发支持和快速的编译速度。",
		TokenCount: 35,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 期望使用自定义模板
	mockClient.EXPECT().
		Generate(mock.Anything, mock.MatchedBy(func(prompt string) bool {
			return strings.Contains(prompt, "请根据以下信息回答问题") &&
				strings.Contains(prompt, "信息源") &&
				strings.Contains(prompt, "答案:")
		}), mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	// 创建自定义模板的RAG服务
	rag := NewRAG(mockClient, WithTemplate(customTemplate))
	ctx := context.Background()
	resp, err := rag.Answer(ctx, question, contexts)

	// 验证结果
	require.NoError(t, err)
	assert.Equal(t, mockResponse.Text, resp.Answer)
}

// TestRAGConfigurationOptions 测试RAG配置选项
func TestRAGConfigurationOptions(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 测试问题和上下文
	question := "简单问题"
	contexts := []string{"简单上下文"}

	// 模拟响应
	mockResponse := &Response{
		Text:       "简单回答",
		TokenCount: 10,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 验证传递配置选项 - 使用任意匹配器，然后在测试结果上验证
	mockClient.EXPECT().
		Generate(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	// 创建配置了选项的RAG服务
	rag := NewRAG(mockClient,
		WithRAGMaxTokens(500),
		WithRAGTemperature(0.2),
	)

	// 调用服务
	ctx := context.Background()
	_, err := rag.Answer(ctx, question, contexts)
	require.NoError(t, err)

	// 可以在这之后访问rag的配置来验证选项是否正确设置
	assert.Equal(t, 500, rag.config.MaxTokens)
	assert.Equal(t, float32(0.2), rag.config.Temperature)
}

// TestRAGErrorHandling 测试RAG错误处理
func TestRAGErrorHandling(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 空问题错误测试
	rag := NewRAG(mockClient)
	ctx := context.Background()
	_, err := rag.Answer(ctx, "", []string{"上下文"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "question cannot be empty")

	// 模拟LLM客户端错误
	mockError := NewLLMError(ErrCodeServerError, "模型服务器错误")
	mockClient.EXPECT().
		Generate(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, mockError)

	_, err = rag.Answer(ctx, "问题", []string{"上下文"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate response")
}

// TestRAGSourceReferences 测试引用来源功能
func TestRAGSourceReferences(t *testing.T) {
	// 创建Mock客户端
	mockClient := NewMockClient(t)

	// 测试问题和上下文
	question := "测试问题"
	contexts := []string{
		"第一段上下文内容",
		"第二段上下文内容",
		"第三段上下文内容",
	}

	// 模拟响应
	mockResponse := &Response{
		Text:       "基于上下文的回答",
		TokenCount: 15,
		ModelName:  "mock-model",
		FinishTime: time.Now(),
	}

	// 设置客户端期望
	mockClient.EXPECT().
		Generate(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	// 测试包含引用源
	ragWithSources := NewRAG(mockClient, WithSources(true))
	ctx := context.Background()
	respWithSources, err := ragWithSources.Answer(ctx, question, contexts)
	require.NoError(t, err)
	assert.Len(t, respWithSources.Sources, 3)

	// 测试不包含引用源
	mockClient.EXPECT().
		Generate(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockResponse, nil)

	ragWithoutSources := NewRAG(mockClient, WithSources(false))
	respWithoutSources, err := ragWithoutSources.Answer(ctx, question, contexts)
	require.NoError(t, err)
	assert.Empty(t, respWithoutSources.Sources)
}

// TestIntegrationRAGWithTongyi 测试RAG与通义千问模型的集成
// 此测试仅在设置环境变量时运行，避免不必要的API调用
func TestIntegrationRAGWithTongyi(t *testing.T) {
	apiKey := os.Getenv("TONGYI_API_KEY")
	if apiKey == "" {
		t.Skip("Haven't set TONGYI_API_KEY environment variable, skipping test")
	}

	// 创建客户端
	client, err := NewTongyiClient(
		WithAPIKey(apiKey),
		WithModel(ModelQwenTurbo), // 使用速度快的模型
		WithTimeout(10*time.Second),
	)
	require.NoError(t, err)

	rag := NewRAG(client,
		WithRAGMaxTokens(50),
		WithRAGTemperature(0.7),
	)

	ctx := context.Background()
	resp, err := rag.Answer(ctx, "1+1等于几?", []string{"1+1=2"})

	// 只验证基本功能，不关注具体内容
	if err != nil {
		t.Logf("API calling error: %v", err)
		t.Skip("Skipping API test")
		return
	}

	assert.NotEmpty(t, resp.Answer)
	assert.Len(t, resp.Sources, 1)
}
