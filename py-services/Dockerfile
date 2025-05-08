# 基础镜像使用Python官方镜像
FROM python:3.9-slim

# 设置工作目录
WORKDIR /app

# 设置环境变量
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONIOENCODING=utf-8 \
    TZ=Asia/Shanghai

# 安装系统依赖（用于PDF处理）
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libffi-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# 先复制requirements.txt并安装依赖，利用Docker缓存机制
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 复制应用代码
COPY . .

# 创建非root用户并设置权限
RUN useradd -m appuser && chown -R appuser:appuser /app
USER appuser

# 定义健康检查
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 \
  CMD python -c "import requests; requests.get('http://localhost:8000/health')" || exit 1

# 暴露端口
EXPOSE 8000

# 启动命令（可以根据需要修改）
CMD ["python", "-m", "worker.main"]