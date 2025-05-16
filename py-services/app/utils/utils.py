from datetime import datetime
import time
from typing import Dict, Any
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


def parse_redis_url(url):
    """解析Redis URL，提取连接参数"""
    # 检查URL前缀
    if not url.startswith("redis://"):
        raise ValueError(f"Unsupported Redis URL format: {url}")

    # 移除redis://前缀
    url = url.replace("redis://", "")

    # 解析认证部分
    if "@" in url:
        auth, rest = url.split("@", 1)
        if ":" in auth:
            _, password = auth.split(":", 1)
        else:
            password = auth
    else:
        password = None
        rest = url

    # 解析主机和端口
    host_port = rest.split("/")[0]
    if ":" in host_port:
        host, port_str = host_port.split(":", 1)
        port = int(port_str)
    else:
        host = host_port
        port = 6379  # 默认Redis端口

    # 解析数据库
    db = 0  # 默认数据库
    db_parts = rest.split("/")[1:]
    if db_parts and db_parts[0]:  # 确保db_parts[0]不为空字符串
        try:
            db = int(db_parts[0])
        except ValueError:
            # 如果无法解析为整数，使用默认值0
            logger.warning(f"Invalid database number in Redis URL, using default (0): {url}")

    return {
        "host": host,
        "port": port,
        "db": db,
        "password": password,
        "decode_responses": True
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


def send_callback(url, data):
    """Send a callback to the specified URL with the provided data."""
    try:
        # Fix the timestamp handling - ensure it has timezone information
        if "timestamp" in data:
            if isinstance(data["timestamp"], str):
                # If it's already a string but missing timezone
                if "Z" not in data["timestamp"] and "+" not in data["timestamp"] and "-" not in data["timestamp"][-6:]:
                    # If it's an ISO format timestamp
                    if "T" in data["timestamp"]:
                        # Remove microseconds if present (handle cases like 2025-05-16T02:00:47.888351)
                        timestamp_parts = data["timestamp"].split(".")
                        base_timestamp = timestamp_parts[0]
                        # Add Z to indicate UTC timezone
                        data["timestamp"] = base_timestamp + "Z"
                    else:
                        # If not ISO format, convert using standard UTC format
                        data["timestamp"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
            elif isinstance(data["timestamp"], datetime.datetime):
                # Convert datetime to string with timezone
                data["timestamp"] = data["timestamp"].strftime("%Y-%m-%dT%H:%M:%SZ")
            else:
                # For any other type, convert to string with Z timezone
                data["timestamp"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
        
        # Ensure status is a string
        if "status" in data and not isinstance(data["status"], str):
            data["status"] = str(data["status"])
            
        # Ensure type is a string
        if "type" in data and not isinstance(data["type"], str):
            data["type"] = str(data["type"])
            
        headers = {"Content-Type": "application/json"}
        response = requests.post(url, json=data, headers=headers, timeout=5)
        
        if response.status_code >= 400:
            logger.error(f"Failed to send callback to {url}: {response.status_code} {response.reason}")
            return False
            
        return True
    except Exception as e:
        logger.error(f"Failed to send callback to {url}: {str(e)}")
        return False


def get_task_key(task_id: str) -> str:
    """
    生成Redis中任务的键

    参数:
        task_id: 任务ID

    返回:
        str: Redis键
    """
    # 防止添加冗余的"task:"前缀
    if task_id.startswith("task:"):
        return task_id
    else:
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