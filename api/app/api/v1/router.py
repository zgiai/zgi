from fastapi import APIRouter
from .route_map import setup_routes

router = APIRouter()

# 使用路由映射设置路由
setup_routes(router)
