import os
import time
from typing import Dict, Any, Optional
from pathlib import Path
import requests
from functools import wraps

from loguru import logger

# 记录器配置
def setup_logger(log_level: str = "INFO") -> None:
    """
    配置日志记录器

    参数:
        log_level: 日志级别, 默认为INFO
    """
    # 移除默认的处理器
    logger.remove()

    # 添加控制台处理器
    logger.add(
        sink=lambda msg: print(msg, end=""),
        level=log_level,
        format="<green>{time:YYYY-MM-DD HH:mm:ss}</green> | <level>{level: <8}</level> | <cyan>{name}</cyan>:<cyan>{function}</cyan>:<cyan>{line}</cyan> - <level>{message}</level>",
    )

    # 添加文件处理器
    log_dir = Path("logs")
    log_dir.mkdir(exist_ok=True)

    logger.add(
        sink=log_dir / "app_{time}.log",
        level=log_level,
        format="{time:YYYY-MM-DD HH:mm:ss} | {level: <8} | {name}:{function}:{line} - {message}",
        rotation="500 MB",    # 日志文件大小达到500MB时轮换
        retention="10 days",  # 保留10天的日志
        compression="zip",    # 压缩旧的日志文件
    )


def parse_redis_url(url: Optional[str] = None) -> Dict[str, Any]:
    """
    解析Redis URL为连接参数

    参数:
        url: Redis URL, 例如: redis://user:password@localhost:6379/0
             如果为None, 则使用环境变量REDIS_URL

    返回:
        Dict[str, Any]: Redis连接参数
    """
    if url is None:
        url = os.getenv("REDIS_URL", "redis://localhost:6379/0")

    # 简单的URL解析
    if "://" in url:
        _, url = url.split("://", 1)

    auth_host, *db_parts = url.split("/")
    db = int(db_parts[0]) if db_parts else 0

    if "@" in auth_host:
        auth, host_port = auth_host.split("@", 1)

        if ":" in auth:
            _, password = auth.split(":", 1)
        else:
            password = auth
    else:
        password = ""
        host_port = auth_host

    if ":" in host_port:
        host, port = host_port.split(":", 1)
        port = int(port)
    else:
        host = host_port
        port = 6379

    return {
        "host": host,
        "port": port,
        "db": db,
        "password": password or None,
        "decode_responses": True,
    }


def retry(max_retries: int = 3, delay: int = 1, backoff: int = 2, exceptions: tuple = (Exception,)):
    """
    函数重试装饰器

    参数:
        max_retries (int): 最大重试次数
        delay (int): 初始延迟(秒)
        backoff (int): 延迟倍数
        exceptions (tuple): 要捕获的异常元组
    """
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            mtries, mdelay = max_retries, delay
            while mtries > 0:
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    mtries -= 1
                    if mtries <= 0:
                        raise

                    msg = f"Retrying {func.__name__} in {mdelay}s due to {e}"
                    logger.warning(msg)

                    time.sleep(mdelay)
                    mdelay *= backoff
        return wrapper
    return decorator


def format_task_info(task: Any) -> Dict[str, Any]:
    """
    格式化任务信息用于日志记录

    参数:
        task: 任务对象

    返回:
        Dict[str, Any]: 格式化后的任务信息
    """
    info = {
        "task_id": getattr(task, "id", "unknown"),
        "type": getattr(task, "type", "unknown"),
        "document_id": getattr(task, "document_id", "unknown"),
        "status": getattr(task, "status", "unknown"),
    }

    if hasattr(task, "error") and task.error:
        info["error"] = task.error

    return info


def send_callback(callback_url: str, data: Dict[str, Any], timeout: int = 10) -> bool:
    """
    发送回调请求到指定URL

    参数:
        callback_url: 回调URL
        data: 要发送的数据
        timeout: 请求超时时间(秒)

    返回:
        bool: 是否成功
    """
    try:
        headers = {"Content-Type": "application/json"}
        response = requests.post(
            url=callback_url,
            json=data,
            headers=headers,
            timeout=timeout
        )
        response.raise_for_status()
        return True
    except Exception as e:
        logger.error(f"Failed to send callback to {callback_url}: {str(e)}")
        return False


def get_task_key(task_id: str) -> str:
    """
    生成Redis中任务的键

    参数:
        task_id: 任务ID

    返回:
        str: Redis键
    """
    return f"task:{task_id}"


def get_document_tasks_key(document_id: str) -> str:
    """
    生成Redis中文档任务集合的键

    参数:
        document_id: 文档ID

    返回:
        str: Redis键
    """
    return f"document_tasks:{document_id}"


def count_words(text: str) -> int:
    """
    计算文本中的单词数

    参数:
        text: 文本内容

    返回:
        int: 单词数
    """
    # 简单的单词计数，可以按需改进
    return len(text.split())


def count_chars(text: str) -> int:
    """
    计算文本中的字符数(不包括空白字符)

    参数:
        text: 文本内容

    返回:
        int: 字符数
    """
    return sum(1 for c in text if not c.isspace())