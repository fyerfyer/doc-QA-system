package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"

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
		minScore:    0.5,            // 默认最低相似度分数
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

// isGreeting 检查问题是否为简单问候语
func isGreeting(question string) bool {
	// 转为小写并去除空格以便更准确匹配
	q := strings.ToLower(strings.TrimSpace(question))

	// 常见问候语列表
	greetings := []string{
		"你好", "您好", "早上好", "下午好", "晚上好", "嗨", "hi", "hello",
		"hey", "嘿", "哈喽", "喂", "在吗", "在么", "在不在",
	}

	// 检查是否完全匹配
	for _, g := range greetings {
		if q == g {
			return true
		}
	}

	// 检查是否为有附加内容的问候语
	// 仅对非常短的内容进行匹配，并且必须以问候语开头
	if len(q) < 8 { // 降低长度限制，从15改为8
		for _, g := range greetings {
			if strings.HasPrefix(q, g+" ") {
				return true
			}
		}
	}

	// 如果包含问号，基本可以确定不是问候语
	if strings.Contains(q, "?") || strings.Contains(q, "？") {
		return false
	}

	return false
}

// handleGreeting 处理问候语
func (s *QAService) handleGreeting(ctx context.Context, question string) (string, error) {
	// 构建简单的问候语提示词
	prompt := "用户向我问候：\"" + question + "\"。请你作为一个有礼貌的助手，用简短友善的语言回应这个问候。"

	// 直接调用LLM生成回应
	response, err := s.llm.Generate(
		ctx,
		prompt,
		llm.WithGenerateMaxTokens(128), // 问候语回复不需要太长
		llm.WithGenerateTemperature(0.7),
	)

	if err != nil {
		return "", fmt.Errorf("failed to generate greeting response: %w", err)
	}

	return response.Text, nil
}

// Answer 回答问题
func (s *QAService) Answer(ctx context.Context, question string) (string, []vectordb.Document, error) {
	if question == "" {
		//fmt.Println("DEBUG: Question is empty")
		return "", nil, fmt.Errorf("question cannot be empty")
	}

	// 检查是否是问候语
	if isGreeting(question) {
		//fmt.Println("DEBUG: Question is a greeting")
		greeting, err := s.handleGreeting(ctx, question)
		if err != nil {
			//fmt.Printf("DEBUG: Failed to generate greeting response: %v\n", err)
			return "", nil, err
		}
		return greeting, nil, nil
	}

	// 1. 尝试从缓存获取
	cacheKey := cache.GenerateCacheKey("qa", question)
	cachedAnswer, found, err := s.cache.Get(cacheKey)
	if err == nil && found {
		fmt.Println("DEBUG: Cache hit for answer")
		// 从缓存中同时获取相关文档
		docsCacheKey := cache.GenerateCacheKey("qa_docs", question)
		docsJson, docsFound, docsErr := s.cache.Get(docsCacheKey)

		var sources []vectordb.Document
		if docsErr == nil && docsFound {
			//fmt.Println("DEBUG: Cache hit for documents")
			// 解析缓存的文档列表
			if err := json.Unmarshal([]byte(docsJson), &sources); err != nil {
				//fmt.Printf("DEBUG: Failed to unmarshal cached documents: %v\n", err)
			} else {
				//fmt.Printf("DEBUG: Unmarshaled %d cached documents\n", len(sources))
			}
		} else {
			//fmt.Println("DEBUG: No cache hit for documents")
		}

		return cachedAnswer, sources, nil
	}

	//fmt.Println("DEBUG: No cache hit, performing vector search")

	// 2. 将问题转换为向量
	vector, err := s.embedder.Embed(ctx, question)
	if err != nil {
		//fmt.Printf("DEBUG: Failed to generate embedding: %v\n", err)
		return "", nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 3. 检索相关文档
	filter := vectordb.SearchFilter{
		MinScore:   s.minScore,
		MaxResults: s.searchLimit,
	}
	//fmt.Printf("DEBUG: Searching with filter - MinScore: %f, MaxResults: %d\n", filter.MinScore, filter.MaxResults)
	results, err := s.vectorDB.Search(vector, filter)
	if err != nil {
		//fmt.Printf("DEBUG: Search failed: %v\n", err)
		return "", nil, fmt.Errorf("search failed: %w", err)
	}

	//fmt.Printf("DEBUG: Search returned %d results\n", len(results))

	// 检查是否有高相关度的文档
	hasRelevantDocs := false
	for _, result := range results {
		fmt.Printf("DEBUG: Document score: %f, minScore: %f\n", result.Score, s.minScore)
		if result.Score >= s.minScore {
			hasRelevantDocs = true
			break
		}
	}

	//fmt.Printf("DEBUG: hasRelevantDocs: %v\n", hasRelevantDocs)

	// 如果没有找到高相关度文档，直接用LLM回答
	if len(results) == 0 || !hasRelevantDocs {
		// 构建一个通用知识问答提示词
		prompt := fmt.Sprintf("请基于你的已有知识，回答下面的问题： %s\n如果你不知道问题的答案，回答\"不知道\"", question)

		// 获取LLM的回答
		response, err := s.llm.Generate(ctx, prompt,
			llm.WithGenerateMaxTokens(1000),
			llm.WithGenerateTemperature(0.7))

		if err != nil {
			return "", nil, err
		}

		// 返回答案，不包含来源，因为使用的是LLM的通用知识
		return response.Text, []vectordb.Document{}, nil
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
		return "", nil, fmt.Errorf("file ID cannot be empty")
	}

	//fmt.Printf("DEBUG: AnswerWithFile - checking if file exists: %s\n", fileID)

	// 验证文件是否存在的逻辑
	filter := vectordb.SearchFilter{
		FileIDs:    []string{fileID},
		MaxResults: 1,
	}

	// 检查文件是否存在
	results, err := s.vectorDB.Search(make([]float32, s.vectorDB.GetDimension()), filter)
	if err != nil {
		return "", nil, err
	}

	if len(results) == 0 {
		// 添加缺失的返回错误逻辑
		return "", nil, fmt.Errorf("document with ID %s not found", fileID)
	}

	// 检查是否是问候语
	if isGreeting(question) {
		greeting, err := s.handleGreeting(ctx, question)
		if err != nil {
			return "", nil, err
		}
		return greeting, nil, nil
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
	filter = vectordb.SearchFilter{
		FileIDs:    []string{fileID},
		MinScore:   s.minScore,
		MaxResults: s.searchLimit,
	}
	results, err = s.vectorDB.Search(vector, filter)
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

	// 如果没有找到高相关度文档，使用LLM直接回答
	if len(results) == 0 || !hasRelevantDocs {
		// 构建一个通用知识问答提示词
		prompt := fmt.Sprintf("请基于你的已有知识，回答下面的问题： %s\n如果你不知道问题的答案，回答\"不知道\"", question)

		// 获取LLM的回答
		response, err := s.llm.Generate(ctx, prompt,
			llm.WithGenerateMaxTokens(1000),
			llm.WithGenerateTemperature(0.7))

		if err != nil {
			return "", nil, err
		}

		// 返回答案，不包含来源，因为使用的是LLM的通用知识
		return response.Text, []vectordb.Document{}, nil
	}

	// 提取相关文本内容，只保留相关度高于阈值的文档
	var filteredResults []vectordb.SearchResult
	for _, result := range results {
		if result.Score >= s.minScore {
			filteredResults = append(filteredResults, result)
		}
	}

	// 如果过滤后没有文档，使用LLM直接回答
	if len(filteredResults) == 0 {
		prompt := "用户询问了关于特定文件的问题，但我们在文档中未找到足够相关的内容。问题是：" + question
		response, err := s.llm.Generate(
			ctx,
			prompt,
			llm.WithGenerateMaxTokens(512),
		)

		if err != nil {
			// 如果LLM调用失败，返回默认消息
			defaultMsg := "抱歉，在指定文件中没有找到能回答您问题的相关信息。"
			s.cache.Set(cacheKey, defaultMsg, s.cacheTTL)
			return defaultMsg, nil, nil
		}

		// 缓存LLM回答
		s.cache.Set(cacheKey, response.Text, s.cacheTTL)
		return response.Text, nil, nil
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

	// 检查是否是问候语
	if isGreeting(question) {
		greeting, err := s.handleGreeting(ctx, question)
		if err != nil {
			return "", nil, err
		}
		return greeting, nil, nil
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

	// 如果没有找到高相关度文档，使用LLM直接回答
	if len(results) == 0 || !hasRelevantDocs {
		// 构建提示词，指明在特定元数据过滤条件下没找到信息
		metaPrompt := "用户使用特定过滤条件询问问题：" + question +
			"\n\n请告诉用户您在这些特定条件下没有找到相关信息，但可以尝试回答他们的一般性问题。"

		metaResponse, err := s.llm.Generate(
			ctx,
			metaPrompt,
			llm.WithGenerateMaxTokens(512),
			llm.WithGenerateTemperature(0.7),
		)

		if err != nil {
			return "", nil, fmt.Errorf("failed to generate metadata-filtered answer: %w", err)
		}

		// 缓存此结果
		s.cache.Set(cacheKey, metaResponse.Text, s.cacheTTL)

		return metaResponse.Text, nil, nil
	}

	// 提取相关文本内容，只保留相关度高于阈值的文档
	var filteredResults []vectordb.SearchResult
	for _, result := range results {
		if result.Score >= s.minScore {
			filteredResults = append(filteredResults, result)
		}
	}

	// 如果过滤后没有文档，使用LLM直接回答
	if len(filteredResults) == 0 {
		prompt := "用户使用特定元数据筛选条件询问问题，但我们未找到足够相关的内容。问题是：" + question
		response, err := s.llm.Generate(
			ctx,
			prompt,
			llm.WithGenerateMaxTokens(512),
		)

		if err != nil {
			// 如果LLM调用失败，返回默认消息
			defaultMsg := "抱歉，根据您的筛选条件，我没有找到相关信息。"
			s.cache.Set(cacheKey, defaultMsg, s.cacheTTL)
			return defaultMsg, nil, nil
		}

		// 缓存LLM回答
		s.cache.Set(cacheKey, response.Text, s.cacheTTL)
		return response.Text, nil, nil
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

// GetRecentQuestions 获取最近的问题（从聊天历史中获取）
func (s *QAService) GetRecentQuestions(ctx context.Context, limit int) ([]string, error) {
	// 检查参数有效性
	if limit <= 0 {
		limit = 10 // 默认获取10个问题
	}

	// 使用ChatRepository获取最近的消息
	chatRepo := repository.NewChatRepository()
	messages, err := chatRepo.GetRecentMessages(limit * 2) // 获取更多消息，因为不是所有消息都是问题
	if err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}

	// 提取用户问题
	var questions []string
	uniqueQuestions := make(map[string]bool) // 用于去重

	for _, msg := range messages {
		// 只处理用户角色的消息(问题)
		if msg.Role == models.RoleUser {
			question := strings.TrimSpace(msg.Content)
			if question != "" && !uniqueQuestions[question] {
				questions = append(questions, question)
				uniqueQuestions[question] = true

				// 当收集到足够数量的问题时退出
				if len(questions) >= limit {
					break
				}
			}
		}
	}

	return questions, nil
}

// ClearCache 清除问答缓存
func (s *QAService) ClearCache() error {
	return s.cache.Clear()
}
