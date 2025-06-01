import json
from datetime import datetime
from typing import Dict, Any

from fastapi import APIRouter, Request

from app.models.model import TaskType, TaskStatus
from app.utils.utils import logger, get_document_tasks_key
from app.worker.tasks import get_redis_client, get_task_from_redis, update_task_status

# 创建路由器
router = APIRouter(prefix="/api/callback", tags=["callback"])

@router.post("/")
async def handle_callback(request: Request) -> Dict[str, Any]:
    """
    处理任务回调请求

    接收Go服务发送的任务状态更新，并处理相应的后续操作
    """
    try:
        # 解析请求体
        body = await request.json()

        # 提取任务信息
        task_id = body.get('task_id')
        document_id = body.get('document_id')
        status = body.get('status')
        task_type = body.get('type')
        result = body.get('result')
        error = body.get('error', '')

        logger.info(f"Received callback for task {task_id}, document {document_id}, status: {status}")

        # 验证必填字段
        if not task_id or not document_id or not status or not task_type:
            logger.error("Missing required fields in callback request")
            return {
                "success": False,
                "message": "Missing required fields: task_id, document_id, status or type",
                "task_id": task_id or "unknown",
                "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
            }

        # 从Redis获取任务信息
        task = get_task_from_redis(task_id)
        if not task:
            logger.error(f"Task {task_id} not found in Redis")
            return {
                "success": False,
                "message": f"Task {task_id} not found",
                "task_id": task_id,
                "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
            }

        # 解析状态和任务类型
        try:
            task_status = TaskStatus(status)
            task_type_enum = TaskType(task_type)
        except ValueError as e:
            logger.error(f"Invalid task status or type: {str(e)}")
            return {
                "success": False,
                "message": f"Invalid task status or type: {str(e)}",
                "task_id": task_id,
                "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
            }

        # 更新任务状态
        success = update_task_status(task, task_status, result, error)
        if not success:
            return {
                "success": False,
                "message": "Failed to update task status",
                "task_id": task_id,
                "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
            }

        # 如果任务失败，记录详细日志
        if task_status == TaskStatus.FAILED:
            logger.error(f"Task {task_id} failed: {error}")
            # 这里可以添加任务失败的处理逻辑，比如通知管理员、重试等

        # 根据需要发送任务完成的通知到Go服务
        # 这已经由update_task_status函数处理，它会向CALLBACK_URL发送通知

        return {
            "success": True,
            "message": "Callback processed successfully",
            "task_id": task_id,
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }

    except json.JSONDecodeError:
        logger.error("Failed to parse request body as JSON")
        return {
            "success": False,
            "message": "Invalid JSON in request body",
            "task_id": "unknown",
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }
    except Exception as e:
        logger.error(f"Error processing callback: {str(e)}")
        return {
            "success": False,
            "message": f"Error processing callback: {str(e)}",
            "task_id": body.get('task_id', 'unknown') if 'body' in locals() else 'unknown',
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }


@router.get("/task/{task_id}")
async def get_task_status(task_id: str) -> Dict[str, Any]:
    """
    获取任务状态

    根据任务ID查询当前状态
    """
    try:
        # 从Redis获取任务信息
        task = get_task_from_redis(task_id)
        if not task:
            return {
                "success": False,
                "message": f"Task {task_id} not found",
                "task_id": task_id,
                "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
            }

        # 返回任务状态
        return {
            "success": True,
            "task_id": task_id,
            "status": task.status,
            "document_id": task.document_id,
            "created_at": task.created_at.isoformat() if task.created_at else None,
            "updated_at": task.updated_at.isoformat() if task.updated_at else None,
            "error": task.error or None,
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }

    except Exception as e:
        logger.error(f"Error getting task status: {str(e)}")
        return {
            "success": False,
            "message": f"Error getting task status: {str(e)}",
            "task_id": task_id,
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }


@router.get("/document/{document_id}/tasks")
async def get_document_tasks(document_id: str) -> Dict[str, Any]:
    """
    获取文档相关的所有任务

    根据文档ID查询所有相关任务
    """
    try:
        client = get_redis_client()
        key = get_document_tasks_key(document_id)
        task_ids = client.smembers(key)

        tasks = []
        for task_id in task_ids:
            task_id = task_id.decode('utf-8') if isinstance(task_id, bytes) else task_id
            task = get_task_from_redis(task_id)
            if task:
                tasks.append({
                    "task_id": task.id,
                    "type": task.type,
                    "status": task.status,
                    "created_at": task.created_at.isoformat() if task.created_at else None,
                    "updated_at": task.updated_at.isoformat() if task.updated_at else None
                })

        return {
            "success": True,
            "document_id": document_id,
            "tasks": tasks,
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }

    except Exception as e:
        logger.error(f"Error getting document tasks: {str(e)}")
        return {
            "success": False,
            "message": f"Error getting document tasks: {str(e)}",
            "document_id": document_id,
            "timestamp": datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")
        }