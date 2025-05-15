# 确保导入所有相关模块
from . import tasks
from . import processor
from . import celery_app

# 导出celery app实例
app = celery_app.app