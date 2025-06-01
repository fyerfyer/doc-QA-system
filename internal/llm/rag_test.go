package llm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/pyprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestPythonRAGBasicFunctionality 使用Python服务测试RAG基本功能
func TestPythonRAGBasicFunctionality(t *testing.T) {
	// 创建Python服务配置
	pyConfig := &pyprovider.PyServiceConfig{
		BaseURL:    "http://localhost:8000/api",
		Timeout:    30 * time.Second,
		MaxRetries: 2,
	}

	// 创建Python RAG服务
	rag, err := NewRAGWithPython(pyConfig)
	require.NoError(t, err, "failed to create Python RAG service")

	// 测试问题和上下文
	question := "什么是向量数据库？"
	contexts := []string{
		"向量数据库是一种专门存储和检索向量数据的数据库系统。",
		"与传统数据库不同，向量数据库针对高维向量的相似度搜索进行了优化。",
	}

	// 调用RAG服务生成回答
	ctx := context.Background()
	response, err := rag.Answer(ctx, question, contexts)

	require.NoError(t, err, "failed to generate response from Python RAG service")
	assert.NotEmpty(t, response.Answer, "RAG response should not be empty")
	assert.Len(t, response.Sources, 2, "should have two sources")
	assert.Equal(t, contexts[0], response.Sources[0].Content, "sources should match provided contexts")
}

// TestPythonRAGWithOptions 测试Python RAG的不同选项
func TestPythonRAGWithOptions(t *testing.T) {
	// 创建Python服务配置
	pyConfig := &pyprovider.PyServiceConfig{
		BaseURL: "http://localhost:8000/api",
		Timeout: 30 * time.Second,
	}

	// 测试问题和上下文
	question := "Go语言有哪些优势？"
	contexts := []string{
		"Go语言是由Google开发的静态类型编程语言。",
		"Go语言的主要优势包括简洁的语法、强大的并发支持和快速的编译速度。",
	}

	// 测试不同配置的RAG
	t.Run("with citation enabled", func(t *testing.T) {
		rag, err := NewRAGWithPython(pyConfig, WithSources(true))
		require.NoError(t, err, "failed to create Python RAG service with sources enabled")

		ctx := context.Background()
		response, err := rag.Answer(ctx, question, contexts)

		require.NoError(t, err, "failed to generate response with sources enabled")

		assert.NotEmpty(t, response.Answer, "RAG response should not be empty")
		assert.NotEmpty(t, response.Sources, "should have sources")
	})
}

// TestIntegrationRAGWithPythonAPI 集成测试Python RAG API
func TestIntegrationRAGWithPythonAPI(t *testing.T) {
	// 创建Python服务配置
	pyConfig := &pyprovider.PyServiceConfig{
		BaseURL: "http://localhost:8000/api",
		Timeout: 30 * time.Second,
	}

	httpClient, err := pyprovider.NewClient(pyConfig)
	require.NoError(t, err, "failed to create HTTP client for Python service")

	llmClient := pyprovider.NewLLMClient(httpClient)

	// 直接使用PyProvider的LLM客户端
	t.Run("direct python llm test", func(t *testing.T) {
		ctx := context.Background()
		response, err := llmClient.Generate(ctx, "1+1等于几?")

		require.NoError(t, err, "failed to call Generate method on Python LLM client")

		assert.NotEmpty(t, response.Text, "response text should not be empty")
		assert.Contains(t, response.Text, "2", "response should contain the answer to 1+1")
	})

	// 使用Python的RAG功能
	t.Run("direct python rag test", func(t *testing.T) {
		ctx := context.Background()
		response, err := llmClient.Answer(ctx, "Go语言是什么?")

		require.NoError(t, err, "failed to call Answer method on Python LLM client")

		assert.NotEmpty(t, response.Text, "response text should not be empty")
	})
}

// 原始的测试仍保留，但在需要时可以使用Mock客户端
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

// TestIntegrationRAGWithPython 测试RAG与Python模型的集成
func TestIntegrationRAGWithPython(t *testing.T) {
	serviceURL := "http://localhost:8000/api"

	// 创建Python客户端
	client, err := NewPythonClient(
		WithBaseURL(serviceURL),
		WithModel("default"),
		WithTimeout(15*time.Second),
	)
	require.NoError(t, err, "创建Python客户端失败")

	// 使用Python客户端创建RAG服务
	rag := NewRAG(client,
		WithRAGMaxTokens(50),
		WithRAGTemperature(0.7),
	)

	ctx := context.Background()
	resp, err := rag.Answer(ctx, "1+1等于几?", []string{"1+1=2"})

	// 只验证基本功能，不关注具体内容
	if err != nil {
		t.Logf("Python API调用错误: %v", err)
		t.Skip("跳过Python API测试")
		return
	}

	assert.NotEmpty(t, resp.Answer, "response answer should not be empty")
	assert.Len(t, resp.Sources, 1, "should have one source")
}
