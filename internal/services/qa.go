package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fyerfyer/doc-QA-system/internal/models"
	"github.com/fyerfyer/doc-QA-system/internal/repository"
	"strings"
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

	// 检查是否部分匹配 (短问候语可能有附加内容)
	if len(q) < 15 { // 限制长度防止误判
		for _, g := range greetings {
			if strings.HasPrefix(q, g+" ") || strings.Contains(q, g+"，") ||
				strings.Contains(q, g+"!") || strings.Contains(q, g+"！") {
				return true
			}
		}
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

	// 如果没有找到高相关度文档，但有一些相关文档，仍使用它们
	if !hasRelevantDocs && len(results) > 0 {
		// 如果有低相关度文档，使用前几个最相关的
		filteredResults := results
		if len(results) > 3 {
			filteredResults = results[:3] // 取前3个最相关文档
		}

		contexts := make([]string, len(filteredResults))
		sources := make([]vectordb.Document, len(filteredResults))
		for i, result := range filteredResults {
			contexts[i] = result.Document.Text
			sources[i] = result.Document
		}

		// 使用特殊的低置信度提示词
		lowConfidencePrompt := "用户提问：" + question +
			"\n\n我找到了一些可能相关但置信度不高的信息，请尝试根据这些信息回答用户问题，如果信息不足，可以说明这一点。"

		for i, ctx := range contexts {
			lowConfidencePrompt += fmt.Sprintf("\n\n参考信息[%d]: %s", i+1, ctx)
		}

		response, err := s.llm.Generate(
			ctx,
			lowConfidencePrompt,
			llm.WithGenerateMaxTokens(1024),
			llm.WithGenerateTemperature(0.7),
		)

		if err != nil {
			return "", nil, fmt.Errorf("failed to generate low confidence answer: %w", err)
		}

		// 缓存结果
		s.cache.Set(cacheKey, response.Text, s.cacheTTL)

		return response.Text, sources, nil
	}

	// 如果没有找到高相关度文档，直接用LLM回答
	if len(results) == 0 || !hasRelevantDocs {
		// 构建一个通用知识问答提示词
		generalPrompt := "请回答用户的问题：" + question +
			"\n\n如果您不知道答案，请直接说\"抱歉，我没有足够信息回答这个问题。\"不要编造信息。"

		genResponse, err := s.llm.Generate(
			ctx,
			generalPrompt,
			llm.WithGenerateMaxTokens(1024),
			llm.WithGenerateTemperature(0.7),
		)

		if err != nil {
			return "", nil, fmt.Errorf("failed to generate general answer: %w", err)
		}

		// 缓存此结果
		s.cache.Set(cacheKey, genResponse.Text, s.cacheTTL)

		return genResponse.Text, nil, nil
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

	fmt.Printf("DEBUG: AnswerWithFile - checking if file exists: %s\n", fileID)

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
		// 构建一个特定文件问答提示词，但指出在该文件中没找到信息
		filePrompt := "用户正在询问关于特定文件的问题：" + question +
			"\n\n请告诉用户您在这个特定文件中没有找到相关信息，但可以尝试回答他们的一般性问题。"

		fileResponse, err := s.llm.Generate(
			ctx,
			filePrompt,
			llm.WithGenerateMaxTokens(512),
			llm.WithGenerateTemperature(0.7),
		)

		if err != nil {
			return "", nil, fmt.Errorf("failed to generate file-specific answer: %w", err)
		}

		// 缓存此结果
		s.cache.Set(cacheKey, fileResponse.Text, s.cacheTTL)

		return fileResponse.Text, nil, nil
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
