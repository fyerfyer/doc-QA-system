import logging
import time
from typing import List, Dict, Any, Optional, Union, AsyncGenerator

from app.llm.base import BaseLLM
from app.llm.factory import create_llm, get_default_llm
from app.llm.model import RAGRequest, RAGResponse, SearchResult, TokenUsage, LLMError, LLMErrorCode
from app.embedders.factory import create_embedder, get_default_embedder

# 初始化日志记录器
logger = logging.getLogger(__name__)

# 基础RAG提示模板
DEFAULT_RAG_TEMPLATE = """
你是一个专业的助手，可以根据提供的上下文信息回答问题。

请基于以下上下文信息，回答最后的问题：

上下文信息：
{context}

问题：{question}

请仅基于提供的上下文信息回答问题。如果上下文中没有足够的信息，请回答"基于提供的信息，我无法回答这个问题"，并可以建议需要什么额外信息来回答此问题。
"""

# 带有思考步骤的RAG模板
REASONING_RAG_TEMPLATE = """
你是一个专业的助手，可以根据提供的上下文信息回答问题。

请基于以下上下文信息，回答最后的问题：

上下文信息：
{context}

问题：{question}

请先逐步思考这个问题，分析上下文信息，然后仅基于提供的上下文信息回答问题。如果上下文中没有足够的信息，请回答"基于提供的信息，我无法回答这个问题"，并可以建议需要什么额外信息来回答此问题。
"""

# 包含源引用格式的RAG模板
CITATION_RAG_TEMPLATE = """
你是一个专业的助手，可以根据提供的上下文信息回答问题。

请基于以下上下文信息，回答最后的问题：

上下文信息：
{context}

问题：{question}

请仅基于提供的上下文信息回答问题。

当你使用上下文中的信息时，请在句子末尾使用[来源_序号]的形式标注来源，其中序号是上下文块的编号。

如果上下文中没有足够的信息，请回答"基于提供的信息，我无法回答这个问题"，并可以建议需要什么额外信息来回答此问题。
"""


class RAG:
    """检索增强生成类，整合向量搜索和LLM生成来回答基于文档的问题"""
    
    def __init__(
            self,
            llm: Optional[BaseLLM] = None,
            embedder = None,
            vector_store = None,
            template: str = DEFAULT_RAG_TEMPLATE,
            **kwargs
        ):
        """
        初始化RAG实例
        
        参数:
            llm: 语言模型实例，如果不提供则使用默认LLM
            embedder: 嵌入模型实例，如果不提供则使用默认嵌入器
            vector_store: 向量存储实例，如果不提供则尚未实现此功能
            template: RAG提示模板
            **kwargs: 其他配置参数
        """
        # 初始化LLM
        self.llm = llm if llm else get_default_llm()
        
        # 初始化嵌入模型
        self.embedder = embedder if embedder else get_default_embedder()
        
        # 向量存储（目前仅准备接口，实际实现取决于与Go服务的集成）
        self.vector_store = vector_store
        
        # 配置参数
        self.template = template
        self.max_tokens = kwargs.get("max_tokens", 2048)
        self.temperature = kwargs.get("temperature", 0.7)
        self.top_k = kwargs.get("top_k", 5)
        self.enable_citation = kwargs.get("enable_citation", False)
        self.min_relevance_score = kwargs.get("min_relevance_score", 0.6)
        self.enable_reasoning = kwargs.get("enable_reasoning", False)
        self.deduplicate_results = kwargs.get("deduplicate_results", True)
        
        # 如果启用引用，使用引用模板
        if self.enable_citation:
            self.template = CITATION_RAG_TEMPLATE
            
        # 如果启用思考模式，使用思考模板
        if self.enable_reasoning:
            self.template = REASONING_RAG_TEMPLATE
            
        # 日志记录器
        self.logger = logger
        
        self.logger.info(f"Initialized RAG instance with LLM: {self.llm.get_model_name()}")
        
    def set_template(self, template: str) -> None:
        """
        设置RAG提示模板
        
        参数:
            template: 新的提示模板
        """
        self.template = template
        
    def get_template(self) -> str:
        """
        获取当前RAG提示模板
        
        返回:
            str: 当前的提示模板
        """
        return self.template
        
    async def ask_vector_db(
            self, 
            query: str, 
            collection_name: str = None, 
            document_ids: List[str] = None,
            filter_metadata: Dict[str, Any] = None,
            top_k: int = None
        ) -> List[SearchResult]:
        """
        从向量数据库查询相关文档
        
        参数:
            query: 查询文本
            collection_name: 集合名称
            document_ids: 要搜索的文档ID列表
            filter_metadata: 元数据过滤条件
            top_k: 返回结果数量
            
        返回:
            List[SearchResult]: 搜索结果列表
            
        注意:
            此方法需要与Go后端的向量数据库集成，
            现阶段仅提供接口，实际实现会调用后端API
        """
        # TODO: 与Go后端向量数据库集成
        self.logger.warning("Vector DB search not implemented yet, will be integrated with Go backend")
        
        # 返回空列表表示未找到结果
        return []
    
    def format_context(self, search_results: List[SearchResult], add_source_info: bool = False) -> str:
        """
        将搜索结果格式化为上下文字符串
        
        参数:
            search_results: 搜索结果列表
            add_source_info: 是否添加来源信息
            
        返回:
            str: 格式化后的上下文字符串
        """
        if not search_results:
            return "No relevant context information found."
        
        # 去重逻辑（如果启用）
        if self.deduplicate_results:
            seen_texts = set()
            unique_results = []
            for result in search_results:
                if result.text not in seen_texts:
                    seen_texts.add(result.text)
                    unique_results.append(result)
            search_results = unique_results
            
        # 根据相关性得分排序（得分高的在前）
        search_results = sorted(search_results, key=lambda x: x.score, reverse=True)
        
        # 过滤低相关性内容
        if self.min_relevance_score > 0:
            search_results = [r for r in search_results if r.score >= self.min_relevance_score]
            
        # 格式化上下文
        formatted_chunks = []
        for i, result in enumerate(search_results):
            if add_source_info:
                source_info = f"[SOURCE_{i+1}]"
                chunk_text = f"{source_info} {result.text}"
            else:
                chunk_text = f"- {result.text}"
            formatted_chunks.append(chunk_text)
            
        return "\n\n".join(formatted_chunks)
    
    def apply_template(self, question: str, context: str, template: str = None) -> str:
        """
        应用提示模板
        
        参数:
            question: 用户问题
            context: 上下文信息
            template: 可选的自定义模板
            
        返回:
            str: 格式化后的完整提示
        """
        # 使用提供的模板或默认模板
        template_to_use = template or self.template
        
        # 格式化提示
        prompt = template_to_use.format(
            context=context,
            question=question
        )
        
        return prompt
    
    async def generate_answer(
            self, 
            query: str, 
            search_results: List[SearchResult], 
            **kwargs
        ) -> RAGResponse:
        """
        基于检索结果生成回答
        
        参数:
            query: 用户查询
            search_results: 搜索结果列表
            **kwargs: 传递给LLM的其他参数
            
        返回:
            RAGResponse: 生成的回答和元数据
        """
        start_time = time.time()
        
        # 格式化上下文
        add_source_info = self.enable_citation or kwargs.get("enable_citation", False)
        context = self.format_context(search_results, add_source_info=add_source_info)
        
        # 使用自定义模板或默认模板
        template = kwargs.get("template", self.template)
        
        # 应用模板生成完整提示
        prompt = self.apply_template(query, context, template)
        
        # LLM参数
        llm_params = {
            "temperature": kwargs.get("temperature", self.temperature),
            "max_tokens": kwargs.get("max_tokens", self.max_tokens)
        }
        
        # 如果启用了思考模式，添加相应参数
        if self.enable_reasoning or kwargs.get("enable_reasoning", False):
            llm_params["enable_thinking"] = True
        
        try:
            # 生成回答
            self.logger.info(f"Generating answer with {self.llm.__class__.__name__} for query: {query[:50]}...")
            answer = self.llm.generate(prompt, **llm_params)
            
            # 构建响应
            elapsed_time = time.time() - start_time
            self.logger.info(f"Answer generated in {elapsed_time:.2f}s")
            
            # 创建令牌使用统计
            # 注意：这里的token计数只是估算，实际值应从LLM响应中获取
            token_usage = TokenUsage(
                prompt_tokens=len(prompt.split()) * 1.3,  # 粗略估算
                completion_tokens=len(answer.split()) * 1.3,  # 粗略估算
                total_tokens=0  # 将在下一行更新
            )
            token_usage.total_tokens = token_usage.prompt_tokens + token_usage.completion_tokens
            
            response = RAGResponse(
                text=answer,
                sources=search_results[:self.top_k],  # 只包含top_k个源
                model=self.llm.get_model_name(),
                usage=token_usage
            )
            
            return response
            
        except Exception as e:
            error_msg = f"Failed to generate answer: {str(e)}"
            self.logger.error(error_msg)
            raise LLMError(error_msg, LLMErrorCode.API_ERROR)
    
    async def generate_answer_stream(
            self, 
            query: str, 
            search_results: List[SearchResult], 
            **kwargs
        ) -> AsyncGenerator[Dict[str, Any], None]:
        """
        基于检索结果流式生成回答
        
        参数:
            query: 用户查询
            search_results: 搜索结果列表
            **kwargs: 传递给LLM的其他参数
            
        返回:
            AsyncGenerator: 生成流式响应的异步生成器
        """
        start_time = time.time()
        
        # 1. 首先生成一个包含搜索结果的响应
        if search_results:
            # 为每个搜索结果构建简要信息（限制长度）
            search_summaries = []
            for i, result in enumerate(search_results[:self.top_k]):
                # 限制文本长度
                text_preview = result.text[:200] + ("..." if len(result.text) > 200 else "")
                source_info = {
                    "index": i + 1,
                    "text": text_preview,
                    "score": round(result.score, 2),
                    "document_id": result.document_id or ""
                }
                search_summaries.append(source_info)
            
            # 返回搜索结果信息
            yield {
                "type": "search_results",
                "sources": search_summaries,
                "count": len(search_summaries)
            }
        
        # 2. 构建提示并生成回答
        try:
            # 格式化上下文
            add_source_info = self.enable_citation or kwargs.get("enable_citation", False)
            context = self.format_context(search_results, add_source_info=add_source_info)
            
            # 使用自定义模板或默认模板
            template = kwargs.get("template", self.template)
            
            # 应用模板生成完整提示
            prompt = self.apply_template(query, context, template)
            
            # LLM参数
            llm_params = {
                "temperature": kwargs.get("temperature", self.temperature),
                "max_tokens": kwargs.get("max_tokens", self.max_tokens),
                "stream": True  # 确保启用流式输出
            }
            
            # 如果启用了思考模式，添加相应参数
            if self.enable_reasoning or kwargs.get("enable_reasoning", False):
                llm_params["enable_thinking"] = True
            
            # 使用LLM流式生成回答
            self.logger.info(f"Generating streaming answer with {self.llm.__class__.__name__} for query: {query[:50]}...")
            
            # 流式调用LLM
            response_stream = self.llm.generate_stream(prompt, **llm_params)
            
            # 返回LLM的流式回答
            for text_chunk in response_stream:
                # 检查是否提供了取消信号
                if kwargs.get("check_cancelled") and callable(kwargs["check_cancelled"]):
                    if kwargs["check_cancelled"]():
                        self.logger.info("Stream generation was cancelled by client")
                        yield {
                            "type": "cancelled", 
                            "message": "Generation cancelled by client"
                        }
                        return

                # 返回文本片段
                if text_chunk:
                    yield {
                        "type": "content",
                        "text": text_chunk,
                    }
            
            # 完成后添加一个结束标记
            yield {
                "type": "finish",
                "time_taken": round(time.time() - start_time, 2)
            }
            
        except Exception as e:
            error_msg = f"Failed to generate streaming answer: {str(e)}"
            self.logger.error(error_msg)
            yield {
                "type": "error",
                "message": error_msg
            }

    async def query(self, request: Union[RAGRequest, Dict[str, Any]]) -> RAGResponse:
        """
        执行RAG查询
        
        参数:
            request: RAG查询请求
            
        返回:
            RAGResponse: 生成的回答和元数据
        """
        # 确保请求是RAGRequest对象
        if isinstance(request, dict):
            request = RAGRequest(**request)
            
        self.logger.info(f"Processing RAG query: {request.query[:50]}...")
        
        try:
            # 1. 嵌入查询
            query_embedding = self.embedder.embed(request.query)
            
            # 2. 获取搜索结果
            # 注：实际实现应查询向量数据库，此处为接口预留
            search_results = await self.ask_vector_db(
                query=request.query,
                collection_name=request.collection_name,
                document_ids=request.document_ids,
                filter_metadata=request.filter_metadata,
                top_k=request.top_k or self.top_k
            )
            
            # 如果没有找到结果
            if not search_results:
                self.logger.warning(f"No search results found for query: {request.query[:50]}...")
                # 返回一个标准的"未找到信息"回答
                return RAGResponse(
                    text="I don't have enough information to answer this question based on the available documents.",
                    sources=[],
                    model=self.llm.get_model_name(),
                    usage=TokenUsage()
                )
                
            # 3. 生成回答
            llm_params = {
                "temperature": request.temperature,
                "max_tokens": request.max_tokens,
                "enable_citation": self.enable_citation,
                "enable_reasoning": self.enable_reasoning,
                "template": request.context_prompt or self.template
            }
            
            response = await self.generate_answer(
                query=request.query,
                search_results=search_results,
                **llm_params
            )
            
            return response
            
        except Exception as e:
            error_msg = f"RAG query failed: {str(e)}"
            self.logger.error(error_msg)
            raise LLMError(error_msg, LLMErrorCode.API_ERROR)
    
    async def query_stream(self, request: Union[RAGRequest, Dict[str, Any]]) -> AsyncGenerator[Dict[str, Any], None]:
        """
        执行RAG流式查询
        
        参数:
            request: RAG查询请求
            
        返回:
            AsyncGenerator: 生成流式响应的异步生成器
        """
        # 确保请求是RAGRequest对象
        if isinstance(request, dict):
            request = RAGRequest(**request)
            
        self.logger.info(f"Processing RAG streaming query: {request.query[:50]}...")
        
        try:
            # 开始流式查询
            yield {
                "type": "start",
                "query": request.query
            }
            
            # 1. 嵌入查询
            query_embedding = self.embedder.embed(request.query)
            
            # 2. 获取搜索结果
            search_results = await self.ask_vector_db(
                query=request.query,
                collection_name=request.collection_name,
                document_ids=request.document_ids,
                filter_metadata=request.filter_metadata,
                top_k=request.top_k or self.top_k
            )
            
            # 如果没有找到结果
            if not search_results:
                self.logger.warning(f"No search results found for query: {request.query[:50]}...")
                yield {
                    "type": "content",
                    "text": "抱歉，我在可用的文档中没有找到相关信息来回答这个问题。"
                }
                yield {"type": "finish", "time_taken": 0}
                return
                
            # 3. 流式生成回答
            llm_params = {
                "temperature": request.temperature,
                "max_tokens": request.max_tokens,
                "enable_citation": self.enable_citation,
                "enable_reasoning": self.enable_reasoning,
                "template": request.context_prompt or self.template
            }
            
            # 如果提供了取消检查函数，添加到参数中
            if hasattr(request, "check_cancelled") and callable(request.check_cancelled):
                llm_params["check_cancelled"] = request.check_cancelled
            
            # 流式生成回答并转发响应
            async for chunk in self.generate_answer_stream(
                query=request.query,
                search_results=search_results,
                **llm_params
            ):
                yield chunk
                
        except Exception as e:
            error_msg = f"RAG streaming query failed: {str(e)}"
            self.logger.error(error_msg)
            yield {
                "type": "error",
                "message": error_msg
            }


# 便捷函数：创建RAG实例
def create_rag(
        llm_model: str = "default",
        embedder_model: str = "default",
        template: str = DEFAULT_RAG_TEMPLATE,
        **kwargs
    ) -> RAG:
    """
    创建RAG实例的便捷函数
    
    参数:
        llm_model: LLM模型名称或类型
        embedder_model: 嵌入模型名称或类型
        template: RAG提示模板
        **kwargs: 其他配置参数
        
    返回:
        RAG: 初始化的RAG实例
    """
    # 创建LLM
    llm = create_llm(llm_model)
    
    # 创建嵌入器
    embedder = create_embedder(embedder_model)
    
    # 创建RAG实例
    return RAG(llm=llm, embedder=embedder, template=template, **kwargs)