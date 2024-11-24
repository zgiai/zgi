from fastapi import APIRouter
import importlib
import os
from pathlib import Path

def get_endpoint_modules():
    # 获取 endpoints 目录的路径
    endpoints_dir = Path(__file__).parent / "endpoints"
    modules = []
    
    # 遍历 endpoints 目录下的所有 .py 文件
    for file in endpoints_dir.glob("*.py"):
        if file.stem != "__init__":
            # 导入模块
            module_name = f".endpoints.{file.stem}"
            module = importlib.import_module(module_name, package="app.api.v1")
            if hasattr(module, 'router'):
                # 添加到路由映射
                modules.append({
                    "router": module.router,
                    "prefix": f"/{file.stem}",
                    "tags": [file.stem]
                })
    return modules

def setup_routes(app: APIRouter):
    # 获取所有路由模块
    route_map = get_endpoint_modules()
    
    # 注册路由
    for route in route_map:
        app.include_router(
            router=route["router"],
            prefix=route["prefix"],
            tags=route["tags"]
        )
