import logging
import time
import json
import asyncio
from typing import Dict, List, Any, Optional
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

from app.llm.factory import create_llm
from app.llm.model import RAGRequest, LLMError

from app.llm.rag import create_rag

# 初始化日志记录器
logger = logging.getLogger(__name__)

# 创建路由器
router = APIRouter(prefix="/api/python/llm", tags=["llm"])

# 请求模型定义
class GenerateTextRequest(BaseModel):
    """文本生成请求模型"""
    prompt: str
    model: Optional[str] = "default"  # 默认使用配置的模型
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    stop: Optional[List[str]] = None

class ChatMessageRequest(BaseModel):
    """聊天消息请求模型"""
    role: str
    content: str
    name: Optional[str] = None

class ChatCompletionRequest(BaseModel):
    """聊天完成请求模型"""
    messages: List[ChatMessageRequest]
    model: Optional[str] = "default"
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    stop: Optional[List[str]] = None

class RAGTextRequest(BaseModel):
    """RAG文本生成请求模型"""
    query: str
    document_ids: Optional[List[str]] = None
    collection_name: Optional[str] = None
    top_k: int = 5
    model: Optional[str] = "default"
    temperature: float = 0.7
    max_tokens: int = 2048
    stream: bool = False
    enable_citation: bool = False
    enable_reasoning: bool = False

# 响应模型定义
class GenerateTextResponse(BaseModel):
    """文本生成响应模型"""
    text: str
    model: str
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0
    finish_reason: Optional[str] = None
    processing_time: float = 0

class RAGTextResponse(BaseModel):
    """RAG文本生成响应模型"""
    text: str
    sources: List[Dict[str, Any]] = []
    model: str
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0
    finish_reason: Optional[str] = None
    processing_time: float = 0

@router.post("/generate", response_model=GenerateTextResponse)
async def generate_text(request: GenerateTextRequest):
    """
    生成文本回复
    
    根据提供的提示生成文本回复
    """
    start_time = time.time()
    logger.info(f"Received generate request for model: {request.model}")
    
    try:
        # 如果请求流式响应，返回StreamingResponse
        if request.stream:
            return await generate_text_stream(request)
            
        # 创建LLM实例
        llm = create_llm(request.model)
        
        # 调用LLM生成文本
        text = llm.generate(
            prompt=request.prompt,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
            stop=request.stop
        )
        
        # 构建响应
        response = GenerateTextResponse(
            text=text,
            model=llm.get_model_name(),
            prompt_tokens=int(len(request.prompt.split()) * 1.3),  # 粗略估算token数
            completion_tokens=int(len(text.split()) * 1.3),  # 粗略估算token数
            processing_time=time.time() - start_time
        )
        response.total_tokens = response.prompt_tokens + response.completion_tokens
        
        logger.info(f"Generated text in {response.processing_time:.2f}s")
        return response
        
    except LLMError as e:
        logger.error(f"LLM error during text generation: {e.message}")
        raise HTTPException(status_code=500, detail=f"LLM error: {e.message}")
    except Exception as e:
        logger.error(f"Unexpected error during text generation: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error generating text: {str(e)}")

@router.post("/chat", response_model=GenerateTextResponse)
async def chat_completion(request: ChatCompletionRequest):
    """
    聊天完成接口
    
    根据聊天消息历史生成回复
    """
    start_time = time.time()
    logger.info(f"Received chat request for model: {request.model}")
    
    try:
        # 如果请求流式响应，返回StreamingResponse
        if request.stream:
            return await chat_stream(request)
            
        # 创建LLM实例
        llm = create_llm(request.model)
        
        # 转换消息格式
        messages = []
        for msg in request.messages:
            messages.append({
                "role": msg.role,
                "content": msg.content,
                "name": msg.name
            })
        
        # 调用LLM聊天接口
        text = llm.chat(
            messages=messages,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
            stop=request.stop
        )
        
        # 构建响应
        response = GenerateTextResponse(
            text=text,
            model=llm.get_model_name(),
            prompt_tokens=int(sum(len(msg.content.split()) for msg in request.messages) * 1.3),  # 粗略估算
            completion_tokens=int(len(text.split()) * 1.3),  # 粗略估算
            processing_time=time.time() - start_time
        )
        response.total_tokens = response.prompt_tokens + response.completion_tokens
        
        logger.info(f"Generated chat response in {response.processing_time:.2f}s")
        return response
        
    except LLMError as e:
        logger.error(f"LLM error during chat completion: {e.message}")
        raise HTTPException(status_code=500, detail=f"LLM error: {e.message}")
    except Exception as e:
        logger.error(f"Unexpected error during chat completion: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error generating chat response: {str(e)}")

@router.post("/rag", response_model=RAGTextResponse)
async def generate_rag(request: RAGTextRequest):
    """
    使用RAG(检索增强生成)生成回答
    
    基于文档检索结果生成增强回答
    """
    start_time = time.time()
    logger.info(f"Received RAG request: {request.query[:50]}...")
    
    try:
        # 如果请求流式响应，返回StreamingResponse
        if request.stream:
            return await generate_rag_stream(request)
        
        # 创建RAG实例
        rag_instance = create_rag(
            llm_model=request.model,
            enable_citation=request.enable_citation,
            enable_reasoning=request.enable_reasoning
        )
        
        # 转换为内部请求格式
        internal_request = RAGRequest(
            query=request.query,
            document_ids=request.document_ids,
            collection_name=request.collection_name,
            top_k=request.top_k,
            model=request.model,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
            stream=request.stream
        )
        
        # 调用RAG生成回答
        rag_response = await rag_instance.query(internal_request)
        
        # 格式化来源信息
        sources = []
        for src in rag_response.sources:
            source_info = {
                "text": src.text[:200] + ("..." if len(src.text) > 200 else ""),  # 截断长文本
                "score": src.score,
                "document_id": src.document_id or "",
                "metadata": src.metadata or {}
            }
            sources.append(source_info)
        
        # 构建响应
        response = RAGTextResponse(
            text=rag_response.text,
            sources=sources,
            model=rag_response.model,
            prompt_tokens=int(rag_response.usage.prompt_tokens),
            completion_tokens=int(rag_response.usage.completion_tokens),
            total_tokens=int(rag_response.usage.total_tokens),
            finish_reason=rag_response.finish_reason,
            processing_time=time.time() - start_time
        )
        
        logger.info(f"Generated RAG answer in {response.processing_time:.2f}s")
        return response
        
    except LLMError as e:
        logger.error(f"LLM error during RAG generation: {e.message}")
        raise HTTPException(status_code=500, detail=f"LLM error: {e.message}")
    except Exception as e:
        logger.error(f"Unexpected error during RAG generation: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Error generating RAG response: {str(e)}")

async def generate_text_stream(request: GenerateTextRequest):
    """
    流式生成文本
    
    以流的形式返回生成的文本
    """
    logger.info(f"Starting text stream generation for model: {request.model}")
    
    try:
        # 创建LLM实例
        llm = create_llm(request.model)
        
        # 定义流式响应生成器
        async def stream_generator():
            try:
                # 获取流式生成器
                text_generator = llm.generate_stream(
                    prompt=request.prompt,
                    temperature=request.temperature,
                    max_tokens=request.max_tokens,
                    stop=request.stop
                )
                
                # 按照SSE格式输出每个文本块
                for text_chunk in text_generator:
                    if text_chunk:
                        # 将数据转换为JSON格式
                        data = {
                            "text": text_chunk,
                            "type": "content"
                        }
                        yield f"data: {json.dumps(data)}\n\n"
                        await asyncio.sleep(0.01)  # 避免过快生成导致的阻塞
                    
                # 流结束标记
                yield f"data: {json.dumps({'type': 'finish'})}\n\n"
                yield "data: [DONE]\n\n"
            except Exception as e:
                error_msg = f"Stream generation error: {str(e)}"
                logger.error(error_msg)
                yield f"data: {json.dumps({'type': 'error', 'message': error_msg})}\n\n"
                yield "data: [DONE]\n\n"
                
        # 返回流式响应
        return StreamingResponse(
            stream_generator(),
            media_type="text/event-stream"
        )
        
    except Exception as e:
        logger.error(f"Failed to initialize streaming response: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Stream initialization error: {str(e)}")

async def chat_stream(request: ChatCompletionRequest):
    """
    流式聊天
    
    以流的形式返回聊天回复
    """
    logger.info(f"Starting chat stream for model: {request.model}")
    
    try:
        # 创建LLM实例
        llm = create_llm(request.model)
        
        # 转换消息格式
        messages = []
        for msg in request.messages:
            messages.append({
                "role": msg.role,
                "content": msg.content,
                "name": msg.name if msg.name else None
            })
        
        # 定义流式响应生成器
        async def stream_generator():
            try:
                # 获取流式生成器
                chat_generator = llm.chat_stream(
                    messages=messages,
                    temperature=request.temperature,
                    max_tokens=request.max_tokens,
                    stop=request.stop
                )
                
                # 按照SSE格式输出每个文本块
                for text_chunk in chat_generator:
                    if text_chunk:
                        # 将数据转换为JSON格式
                        data = {
                            "text": text_chunk,
                            "type": "content"
                        }
                        yield f"data: {json.dumps(data)}\n\n"
                        await asyncio.sleep(0.01)  # 避免过快生成导致的阻塞
                    
                # 流结束标记
                yield f"data: {json.dumps({'type': 'finish'})}\n\n"
                yield "data: [DONE]\n\n"
            except Exception as e:
                error_msg = f"Stream chat error: {str(e)}"
                logger.error(error_msg)
                yield f"data: {json.dumps({'type': 'error', 'message': error_msg})}\n\n"
                yield "data: [DONE]\n\n"
                
        # 返回流式响应
        return StreamingResponse(
            stream_generator(),
            media_type="text/event-stream"
        )
        
    except Exception as e:
        logger.error(f"Failed to initialize chat streaming response: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Stream initialization error: {str(e)}")

@router.post("/rag/stream")
async def generate_rag_stream(request: RAGTextRequest):
    """
    流式RAG生成
    
    以流的形式返回RAG生成的回答
    """
    logger.info(f"Starting RAG streaming for query: {request.query[:50]}...")
    
    try:
        # 创建取消检查标志和函数
        is_cancelled = {"value": False}
        
        def check_cancelled():
            return is_cancelled["value"]
        
        # 创建RAG实例
        rag_instance = create_rag(
            llm_model=request.model,
            enable_citation=request.enable_citation,
            enable_reasoning=request.enable_reasoning
        )
        
        # 转换为内部请求格式
        internal_request = RAGRequest(
            query=request.query,
            document_ids=request.document_ids,
            collection_name=request.collection_name,
            top_k=request.top_k,
            model=request.model,
            temperature=request.temperature,
            max_tokens=request.max_tokens,
            stream=True
        )
        
        # 添加取消检查函数
        internal_request.check_cancelled = check_cancelled
        
        # 定义流式响应生成器
        async def stream_generator():
            try:
                # 获取流式生成器
                async for chunk in rag_instance.query_stream(internal_request):
                    yield f"data: {json.dumps(chunk)}\n\n"
                    await asyncio.sleep(0.01)  # 避免过快生成导致的阻塞
                    
                # 流结束标记
                yield "data: [DONE]\n\n"
            except Exception as e:
                error_msg = f"RAG stream generation error: {str(e)}"
                logger.error(error_msg)
                yield f"data: {json.dumps({'type': 'error', 'message': error_msg})}\n\n"
                yield "data: [DONE]\n\n"
        
        # 创建一个包含客户端断开连接处理的响应
        async def stream_with_disconnect_handling(request: Request):
            disconnect_monitor_task = None
            
            async def monitor_client_connection():
                while True:
                    if await request.is_disconnected():
                        # 客户端断开连接，设置取消标志
                        is_cancelled["value"] = True
                        logger.info("Client disconnected, setting cancel flag")
                        break
                    await asyncio.sleep(0.5)  # 检查间隔
            
            try:
                # 启动监控任务
                disconnect_monitor_task = asyncio.create_task(monitor_client_connection())
                
                # 返回生成器内容
                async for chunk in stream_generator():
                    yield chunk
            finally:
                # 清理监控任务
                if disconnect_monitor_task:
                    disconnect_monitor_task.cancel()
        
        # 返回流式响应
        return StreamingResponse(
            stream_with_disconnect_handling(request),
            media_type="text/event-stream"
        )
        
    except Exception as e:
        logger.error(f"Failed to initialize RAG streaming response: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Stream initialization error: {str(e)}")

# 中断请求的API端点
@router.post("/cancel/{request_id}")
async def cancel_streaming(request_id: str):
    """
    取消正在进行的流式生成
    
    参数:
        request_id: 请求ID
    """
    logger.info(f"Request to cancel stream: {request_id}")
    
    # TODO: 实现通过请求ID跟踪和取消请求的功能
    # 在一个实际系统中，我们可以使用Redis或其他存储来跟踪活动的请求
    
    return {
        "success": True, 
        "message": f"Cancel signal sent for request {request_id}"
    }