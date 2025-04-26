package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/cache"
	"github.com/fyerfyer/doc-QA-system/internal/embedding"
	"github.com/fyerfyer/doc-QA-system/internal/llm"
	"github.com/fyerfyer/doc-QA-system/internal/vectordb"
)

// QAService 问答服务
// 负责协调向量检索和大模型生成答案
type QAService struct {
	embedder    embedding.Client    // 嵌入模型客户端
	vectorDB    vectordb.Repository // 向量数据库
	llm         llm.Client          // 大模型客户端
	rag         *llm.RAGService     // RAG服务
	cache       cache.Cache         // 缓存
	cacheTTL    time.Duration       // 缓存有效期
	searchLimit int                 // 搜索结果数量限制
	minScore    float32             // 最低相似度分数
}

// QAOption 问答服务配置选项
type QAOption func(*QAService)

// NewQAService 创建问答服务实例
func NewQAService(
	embedder embedding.Client,
	vectorDB vectordb.Repository,
	llmClient llm.Client,
	rag *llm.RAGService,
	cache cache.Cache,
	opts ...QAOption,
) *QAService {
	// 创建服务实例
	service := &QAService{
		embedder:    embedder,
		vectorDB:    vectorDB,
		llm:         llmClient,
		rag:         rag,
		cache:       cache,
		cacheTTL:    24 * time.Hour, // 默认缓存24小时
		searchLimit: 5,              // 默认检索5个相关文档
		minScore:    0.7,            // 默认最低相似度分数
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// WithCacheTTL 设置缓存时间
func WithCacheTTL(ttl time.Duration) QAOption {
	return func(s *QAService) {
		s.cacheTTL = ttl
	}
}

// WithSearchLimit 设置搜索结果数量
func WithSearchLimit(limit int) QAOption {
	return func(s *QAService) {
		s.searchLimit = limit
	}
}

// WithMinScore 设置最低相似度分数
func WithMinScore(score float32) QAOption {
	return func(s *QAService) {
		s.minScore = score
	}
}

// Answer 回答问题
func (s *QAService) Answer(ctx context.Context, question string) (string, []vectordb.Document, error) {
	if question == "" {
		return "", nil, fmt.Errorf("question cannot be empty")
	}

	// 1. 尝试从缓存获取
	cacheKey := cache.GenerateCacheKey("qa", question)
	cachedAnswer, found, err := s.cache.Get(cacheKey)
	if err == nil && found {
		// 从缓存中同时获取相关文档
		docsCacheKey := cache.GenerateCacheKey("qa_docs", question)
		docsJson, docsFound, docsErr := s.cache.Get(docsCacheKey)

		var sources []vectordb.Document
		if docsErr == nil && docsFound {
			// 解析缓存的文档列表
			if err := json.Unmarshal([]byte(docsJson), &sources); err != nil {
				// 解析失败就使用空列表，不影响主流程
				fmt.Printf("Failed to unmarshal cached documents: %v\n", err)
			}
		}

		return cachedAnswer, sources, nil
	}

	// 2. 将问题转换为向量
	vector, err := s.embedder.Embed(ctx, question)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 3. 检索相关文档
	filter := vectordb.SearchFilter{
		MinScore:   s.minScore,
		MaxResults: s.searchLimit,
	}
	results, err := s.vectorDB.Search(vector, filter)
	if err != nil {
		return "", nil, fmt.Errorf("search failed: %w", err)
	}

	// 检查是否有高相关度的文档
	hasRelevantDocs := false
	for _, result := range results {
		if result.Score >= s.minScore {
			hasRelevantDocs = true
			break
		}
	}

	// 如果没有找到高相关度文档，返回没有找到的消息
	if len(results) == 0 || !hasRelevantDocs {
		noContextAnswer := "抱歉，我没有找到相关信息可以回答您的问题。"
		// 缓存此结果
		s.cache.Set(cacheKey, noContextAnswer, s.cacheTTL)
		return noContextAnswer, nil, nil
	}

	// 4. 提取相关文本内容，只保留相关度高于阈值的文档
	var filteredResults []vectordb.SearchResult
	for _, result := range results {
		if result.Score >= s.minScore {
			filteredResults = append(filteredResults, result)
		}
	}

	// 如果过滤后没有文档，返回没有找到的消息
	if len(filteredResults) == 0 {
		noContextAnswer := "抱歉，我没有找到相关信息可以回答您的问题。"
		// 缓存此结果
		s.cache.Set(cacheKey, noContextAnswer, s.cacheTTL)
		return noContextAnswer, nil, nil
	}

	contexts := make([]string, len(filteredResults))
	sources := make([]vectordb.Document, len(filteredResults))
	for i, result := range filteredResults {
		contexts[i] = result.Document.Text
		sources[i] = result.Document
	}

	// 5. 使用RAG生成回答
	ragResponse, err := s.rag.Answer(ctx, question, contexts)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	// 6. 缓存结果
	s.cache.Set(cacheKey, ragResponse.Answer, s.cacheTTL)

	// 缓存文档列表
	docsCacheKey := cache.GenerateCacheKey("qa_docs", question)
	docsJson, err := json.Marshal(sources)
	if err == nil {
		s.cache.Set(docsCacheKey, string(docsJson), s.cacheTTL)
	}

	return ragResponse.Answer, sources, nil
}

// AnswerWithFile 针对特定文件回答问题
func (s *QAService) AnswerWithFile(ctx context.Context, question string, fileID string) (string, []vectordb.Document, error) {
	if question == "" {
		return "", nil, fmt.Errorf("question cannot be empty")
	}

	if fileID == "" {
		return "", nil, fmt.Errorf("fileID cannot be empty")
	}

	// 特定文件的缓存键
	cacheKey := cache.GenerateCacheKey("qa_file", fileID, question)
	cachedAnswer, found, err := s.cache.Get(cacheKey)
	if err == nil && found {
		// 从缓存中获取文档
		docsCacheKey := cache.GenerateCacheKey("qa_file_docs", fileID, question)
		docsJson, docsFound, docsErr := s.cache.Get(docsCacheKey)

		var sources []vectordb.Document
		if docsErr == nil && docsFound {
			if err := json.Unmarshal([]byte(docsJson), &sources); err != nil {
				fmt.Printf("Failed to unmarshal cached file documents: %v\n", err)
			}
		}

		return cachedAnswer, sources, nil
	}

	// 将问题转换为向量
	vector, err := s.embedder.Embed(ctx, question)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 检索特定文件中的相关文档
	filter := vectordb.SearchFilter{
		FileIDs:    []string{fileID},
		MinScore:   s.minScore,
		MaxResults: s.searchLimit,
	}
	results, err := s.vectorDB.Search(vector, filter)
	if err != nil {
		return "", nil, fmt.Errorf("search failed: %w", err)
	}

	// 检查是否有高相关度的文档
	hasRelevantDocs := false
	for _, result := range results {
		if result.Score >= s.minScore {
			hasRelevantDocs = true
			break
		}
	}

	// 如果没有找到高相关度文档，返回没有找到的消息
	if len(results) == 0 || !hasRelevantDocs {
		noContextAnswer := "抱歉，我没有找到相关信息可以回答您的问题。"
		// 缓存此结果
		s.cache.Set(cacheKey, noContextAnswer, s.cacheTTL)
		return noContextAnswer, nil, nil
	}

	// 提取相关文本内容，只保留相关度高于阈值的文档
	var filteredResults []vectordb.SearchResult
	for _, result := range results {
		if result.Score >= s.minScore {
			filteredResults = append(filteredResults, result)
		}
	}

	// 如果过滤后没有文档，返回没有找到的消息
	if len(filteredResults) == 0 {
		noContextAnswer := "抱歉，在指定文件中没有找到能回答您问题的相关信息。"
		s.cache.Set(cacheKey, noContextAnswer, s.cacheTTL)
		return noContextAnswer, nil, nil
	}

	contexts := make([]string, len(filteredResults))
	sources := make([]vectordb.Document, len(filteredResults))
	for i, result := range filteredResults {
		contexts[i] = result.Document.Text
		sources[i] = result.Document
	}

	// 使用RAG生成回答
	ragResponse, err := s.rag.Answer(ctx, question, contexts)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	// 缓存结果
	s.cache.Set(cacheKey, ragResponse.Answer, s.cacheTTL)

	// 缓存文档列表
	docsCacheKey := cache.GenerateCacheKey("qa_file_docs", fileID, question)
	docsJson, err := json.Marshal(sources)
	if err == nil {
		s.cache.Set(docsCacheKey, string(docsJson), s.cacheTTL)
	}

	return ragResponse.Answer, sources, nil
}

// AnswerWithMetadata 使用元数据过滤回答问题
func (s *QAService) AnswerWithMetadata(ctx context.Context, question string, metadata map[string]interface{}) (string, []vectordb.Document, error) {
	if question == "" {
		return "", nil, fmt.Errorf("question cannot be empty")
	}

	// 创建元数据缓存键
	metadataKey := ""
	for k, v := range metadata {
		metadataKey += fmt.Sprintf("%s:%v;", k, v)
	}
	cacheKey := cache.GenerateCacheKey("qa_meta", metadataKey, question)

	cachedAnswer, found, err := s.cache.Get(cacheKey)
	if err == nil && found {
		// 从缓存中获取文档
		docsCacheKey := cache.GenerateCacheKey("qa_meta_docs", metadataKey, question)
		docsJson, docsFound, docsErr := s.cache.Get(docsCacheKey)

		var sources []vectordb.Document
		if docsErr == nil && docsFound {
			if err := json.Unmarshal([]byte(docsJson), &sources); err != nil {
				fmt.Printf("Failed to unmarshal cached metadata documents: %v\n", err)
			}
		}

		return cachedAnswer, sources, nil
	}

	// 将问题转换为向量
	vector, err := s.embedder.Embed(ctx, question)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 检索带元数据过滤的相关文档
	filter := vectordb.SearchFilter{
		Metadata:   metadata,
		MinScore:   s.minScore,
		MaxResults: s.searchLimit,
	}
	results, err := s.vectorDB.Search(vector, filter)
	if err != nil {
		return "", nil, fmt.Errorf("search failed: %w", err)
	}

	// 检查是否有高相关度的文档
	hasRelevantDocs := false
	for _, result := range results {
		if result.Score >= s.minScore {
			hasRelevantDocs = true
			break
		}
	}

	// 如果没有找到高相关度文档，返回没有找到的消息
	if len(results) == 0 || !hasRelevantDocs {
		noContextAnswer := "抱歉，我没有找到相关信息可以回答您的问题。"
		// 缓存此结果
		s.cache.Set(cacheKey, noContextAnswer, s.cacheTTL)
		return noContextAnswer, nil, nil
	}

	// 提取相关文本内容，只保留相关度高于阈值的文档
	var filteredResults []vectordb.SearchResult
	for _, result := range results {
		if result.Score >= s.minScore {
			filteredResults = append(filteredResults, result)
		}
	}

	// 如果过滤后没有文档，返回没有找到的消息
	if len(filteredResults) == 0 {
		noContextAnswer := "抱歉，根据您的筛选条件，我没有找到相关信息。"
		s.cache.Set(cacheKey, noContextAnswer, s.cacheTTL)
		return noContextAnswer, nil, nil
	}

	contexts := make([]string, len(filteredResults))
	sources := make([]vectordb.Document, len(filteredResults))
	for i, result := range filteredResults {
		contexts[i] = result.Document.Text
		sources[i] = result.Document
	}

	// 使用RAG生成回答
	ragResponse, err := s.rag.Answer(ctx, question, contexts)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	// 缓存结果
	s.cache.Set(cacheKey, ragResponse.Answer, s.cacheTTL)

	// 缓存文档列表
	docsCacheKey := cache.GenerateCacheKey("qa_meta_docs", metadataKey, question)
	docsJson, err := json.Marshal(sources)
	if err == nil {
		s.cache.Set(docsCacheKey, string(docsJson), s.cacheTTL)
	}

	return ragResponse.Answer, sources, nil
}

// GetRecentQuestions 获取最近的问题（假设在另一个存储中保存了问题历史）
func (s *QAService) GetRecentQuestions(ctx context.Context, limit int) ([]string, error) {
	// TODO: 实现获取历史问题的功能
	// 这需要一个专门记录用户问题历史的组件
	// MVP版本先返回空列表
	return []string{}, nil
}

// ClearCache 清除问答缓存
func (s *QAService) ClearCache() error {
	return s.cache.Clear()
}
