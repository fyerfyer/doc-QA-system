"""
任务注册测试脚本
用于验证Celery任务是否正确注册
"""
import sys
import os
import json
from app.worker.celery_app import app
from app.utils.utils import setup_logger, logger

# 初始化日志
setup_logger("INFO")

def test_celery_connection():
    """测试Celery连接"""
    try:
        print("Testing Celery connection to broker...")
        i = app.control.inspect()
        stats = i.stats()
        if stats:
            print("✅ Celery broker connection successful")
            print(f"Connected workers: {', '.join(stats.keys())}")
        else:
            print("❌ No Celery workers are running")
        return stats is not None
    except Exception as e:
        print(f"❌ Failed to connect to Celery broker: {e}")
        return False


def list_registered_tasks():
    """列出所有注册的任务"""
    print("\nListing registered tasks:")
    all_tasks = app.tasks
    user_tasks = [task for task in sorted(all_tasks) if not task.startswith('celery.')]
    
    if not user_tasks:
        print("❌ No user tasks registered!")
        return False
    
    for task_name in user_tasks:
        print(f"- {task_name}")
    
    # 检查关键任务是否注册
    required_tasks = [
        "app.worker.tasks.process_document",
        "app.worker.tasks.parse_document",
        "app.worker.tasks.chunk_text",
        "app.worker.tasks.vectorize_text"
    ]
    
    print("\nVerifying required tasks:")
    all_registered = True
    for task in required_tasks:
        if task in all_tasks:
            print(f"✅ Task {task} is registered")
        else:
            print(f"❌ Task {task} is NOT registered")
            all_registered = False
    
    return all_registered


def inspect_active_workers():
    """检查活跃的worker和队列"""
    print("\nInspecting active workers:")
    i = app.control.inspect()
    
    # 检查已注册的工作节点
    registered = i.registered() or {}
    if registered:
        print(f"Found {len(registered)} registered worker(s):")
        for worker, tasks in registered.items():
            print(f"  - {worker}: {len(tasks)} tasks")
    else:
        print("❌ No registered workers found")
    
    # 检查活跃队列
    active_queues = i.active_queues() or {}
    if active_queues:
        print("\nActive queues:")
        for worker, queues in active_queues.items():
            print(f"  - {worker} listens to:")
            for queue in queues:
                print(f"    - {queue['name']}")
    else:
        print("❌ No active queues found")

    # 检查正在运行的任务
    active = i.active() or {}
    if active:
        print("\nCurrently running tasks:")
        for worker, tasks in active.items():
            print(f"  - {worker}: {len(tasks)} running tasks")
            for task in tasks:
                print(f"    - {task['id']}: {task['name']}")
    else:
        print("\nNo tasks currently running")


def test_task_execution():
    """测试执行一个简单任务"""
    print("\nTesting task execution:")
    
    # 测试清理任务的异步执行
    try:
        result = app.tasks["app.worker.celery_app.cleanup_expired_tasks"].apply_async()
        task_id = result.task_id
        print(f"✅ Task dispatched with ID: {task_id}")
        
        # 等待结果
        print("Waiting for result (5s timeout)...")
        try:
            task_result = result.get(timeout=5)
            print(f"✅ Task result: {task_result}")
            return True
        except Exception as e:
            print(f"❌ Error getting task result: {e}")
            return False
    except Exception as e:
        print(f"❌ Failed to dispatch task: {e}")
        return False


if __name__ == "__main__":
    print("=" * 60)
    print("DocQA Python Worker Task Verification")
    print("=" * 60)
    
    conn_ok = test_celery_connection()
    tasks_ok = list_registered_tasks()
    inspect_active_workers()
    
    if conn_ok and tasks_ok:
        execution_ok = test_task_execution()
    else:
        execution_ok = False
    
    print("\n" + "=" * 60)
    print("Summary:")
    print(f"- Broker Connection: {'✅ OK' if conn_ok else '❌ FAILED'}")
    print(f"- Task Registration: {'✅ OK' if tasks_ok else '❌ FAILED'}")
    print(f"- Task Execution: {'✅ OK' if execution_ok else '❌ FAILED'}")
    print("=" * 60)
    
    if not (conn_ok and tasks_ok and execution_ok):
        print("\n❌ Some tests failed. Please check the configuration.")
        sys.exit(1)
    else:
        print("\n✅ All tests passed. Worker is properly configured.")
        sys.exit(0)