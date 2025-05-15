import json
import os
import importlib
from celery import Celery, signals
from celery.schedules import crontab
from datetime import datetime, timedelta
import redis

from app.utils.utils import setup_logger, parse_redis_url, logger

# 初始化日志
setup_logger(os.getenv("LOG_LEVEL", "INFO"))

# Redis配置
REDIS_URL = os.getenv("REDIS_URL", "redis://localhost:6379/0")
redis_params = parse_redis_url(REDIS_URL)

# 队列配置，对应Go中的队列优先级
task_queues = {
    "critical": {"priority": 6},  # 高优先级队列
    "default": {"priority": 3},   # 默认队列
    "low": {"priority": 1},       # 低优先级队列
}

# Celery配置
app = Celery("docqa_worker")

# 添加任务导入配置，确保任务被自动加载
app.conf.update(
    # 基本配置
    broker_url=REDIS_URL,
    result_backend=REDIS_URL,
    task_serializer="json",
    accept_content=["json"],
    result_serializer="json",
    timezone="UTC",
    enable_utc=True,

    # 队列配置
    task_queues=task_queues,
    task_default_queue="default",

    # 任务路由
    task_routes={
        "app.worker.tasks.process_document": {"queue": "critical"},
        "app.worker.tasks.parse_document": {"queue": "default"},
        "app.worker.tasks.chunk_text": {"queue": "default"},
        "app.worker.tasks.vectorize_text": {"queue": "default"},
    },

    # 添加imports配置确保任务模块被自动加载
    imports=["app.worker.tasks"],

    # 任务执行配置
    task_acks_late=True,           # 任务完成后才确认
    worker_prefetch_multiplier=1,  # 防止一次获取太多任务
    task_reject_on_worker_lost=True, # 工作进程崩溃时，任务重新排队

    # 重试配置
    task_time_limit=3600,          # 任务运行时间限制(秒)
    task_soft_time_limit=3000,     # 软限制时间(秒)

    # 并发和池配置
    worker_concurrency=os.cpu_count(),  # 工作进程数量
    worker_max_tasks_per_child=1000,   # 工作进程处理的最大任务数

    # 添加broker_connection_retry_on_startup配置，解决警告
    broker_connection_retry_on_startup=True,
)

# 手动导入任务模块确保任务被注册
try:
    importlib.import_module("app.worker.tasks")
    logger.info("Successfully imported tasks module")
except ImportError as e:
    logger.error(f"Error importing tasks module: {e}")


# 任务前置处理
@signals.task_prerun.connect
def task_prerun_handler(task_id, task, args, kwargs, **_):
    """任务开始前的处理"""
    logger.info(
        f"Starting task {task.name}[{task_id}] with args: {args} kwargs: {kwargs}"
    )


# 任务后置处理
@signals.task_postrun.connect
def task_postrun_handler(task_id, task, args, kwargs, retval, state, **_):
    """任务完成后的处理"""
    logger.info(
        f"Task {task.name}[{task_id}] finished with state: {state}"
    )


# 任务失败处理
@signals.task_failure.connect
def task_failure_handler(task_id, exception, args, kwargs, traceback, einfo, **_):
    """任务失败的处理"""
    logger.error(
        f"Task {task_id} failed: {exception}\nTraceback: {einfo}"
    )


# Celery Worker启动处理
@signals.worker_ready.connect
def worker_ready_handler(**_):
    """工作进程就绪后的处理"""
    logger.info("Worker is ready to receive tasks")

    # 检查Redis连接
    try:
        r = redis.Redis(**redis_params)
        if r.ping():
            logger.info("Successfully connected to Redis")
        else:
            logger.error("Failed to ping Redis")
    except Exception as e:
        logger.error(f"Error connecting to Redis: {e}")


# 配置周期性任务
@app.on_after_configure.connect
def setup_periodic_tasks(sender, **_):
    """配置周期性任务"""
    # 定期清理过期的任务数据 (每天凌晨2点)
    sender.add_periodic_task(
        crontab(hour=2, minute=0),
        cleanup_expired_tasks.s(),
        name='cleanup-expired-tasks',
    )


# 清理过期任务
@app.task
def cleanup_expired_tasks():
    """清理超过保留期的任务记录和数据"""
    logger.info("Running expired tasks cleanup")

    try:
        # 获取Redis客户端
        redis_client = redis.Redis(**redis_params)

        # 任务保留天数
        retention_days = int(os.getenv("TASK_RETENTION_DAYS", "7"))
        retention_seconds = retention_days * 24 * 60 * 60
        cutoff_time = datetime.now() - timedelta(seconds=retention_seconds)

        # 使用scan迭代查找任务键
        cursor = 0
        task_prefix = "task:"
        document_tasks_prefix = "document_tasks:"
        deleted_count = 0
        processed_count = 0

        while True:
            cursor, keys = redis_client.scan(cursor=cursor, match=f"{task_prefix}*", count=100)
            for key in keys:
                try:
                    key_str = key.decode('utf-8') if isinstance(key, bytes) else key
                    task_id = key_str[len(task_prefix):]  # 提取任务ID
                    task_data = redis_client.get(key_str)
                    processed_count += 1

                    if not task_data:
                        continue

                    task_dict = json.loads(task_data)
                    if 'created_at' in task_dict:
                        created_at_str = task_dict['created_at']
                        try: 
                            # 处理ISO格式时间字符串
                            if 'Z' in created_at_str:
                                created_at_str = created_at_str.replace('Z', '+00:00')
                            created_at = datetime.fromisoformat(created_at_str)
                            
                            # 确保两个时间对象都是aware或naive
                            if created_at.tzinfo is not None and cutoff_time.tzinfo is None:
                                cutoff_time = cutoff_time.replace(tzinfo=created_at.tzinfo)
                            elif created_at.tzinfo is None and cutoff_time.tzinfo is not None:
                                created_at = created_at.replace(tzinfo=cutoff_time.tzinfo)

                            # 任务已过期
                            if created_at < cutoff_time:
                                # 获取文档ID
                                document_id = task_dict.get('document_id')

                                # 从文档任务集合中移除
                                if document_id:
                                    doc_key = f"{document_tasks_prefix}{document_id}"
                                    redis_client.srem(doc_key, task_id)

                                # 删除任务数据
                                redis_client.delete(key_str)
                                deleted_count += 1
                        except Exception as e:
                            logger.error(f"Error processing time for task {task_id}: {str(e)}")

                except Exception as e:
                    logger.error(f"Error processing task key {key}: {str(e)}")

            # 扫描完成
            if cursor == 0:
                break

        logger.info(f"Task cleanup completed: processed {processed_count} tasks, deleted {deleted_count} expired tasks")
        return True
    except Exception as e:
        logger.error(f"Task cleanup failed: {str(e)}")
        return False

# 允许使用app在其他模块中导入Celery实例
if __name__ == "__main__":
    app.start()