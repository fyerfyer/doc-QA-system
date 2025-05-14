import os
from typing import List
from fastapi import FastAPI

def print_routes(app: FastAPI, logger=None):
    """
    打印所有API路由，类似于Gin框架的路由显示
    
    参数:
        app: FastAPI应用实例
        logger: 可选的日志记录器，如果未提供则使用print
    """
    host = os.getenv("HOST", "0.0.0.0")
    port = int(os.getenv("PORT", "8000"))
    base_url = f"http://{host}:{port}"
    
    header = "Available API Routes:"
    separator = "=" * 50
    
    log = logger.info if logger else print
    
    log(header)
    log(separator)
    
    # 对路由进行排序
    routes = sorted(app.routes, key=lambda route: getattr(route, "path", ""))
    
    for route in routes:
        path = getattr(route, "path", None)
        if path:
            methods = getattr(route, "methods", ["GET"])
            methods_str = ", ".join(methods)
            log(f"{methods_str:10} {base_url}{path}")
    
    log(separator)