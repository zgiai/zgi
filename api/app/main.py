from fastapi import FastAPI, Request, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError
from sqlalchemy.exc import SQLAlchemyError
import traceback
import logging
from app.core.security.ip_whitelist import IPWhitelistMiddleware
from app.core.logging.api_logger import APILoggingMiddleware
from app.core.database import Base, engine

from app.features.auth.client.router import router as auth_router
from app.features.auth.console.router import router as auth_console_router
from app.features.api_keys.client.router import router as api_keys_router
from app.features.teams.client.router import router as teams_router
from app.features.teams.console.router import router as teams_console_router
from app.features.applications.console.router import router as applications_console_router
from app.features.teams.console.member_management.router import router as member_management_router
from app.features.usage.router import router as usage_router
from app.features.users.router import router as users_router

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Create database tables
Base.metadata.create_all(bind=engine)

app = FastAPI(
    title="ZGI API",
    description="ZGI API Documentation",
    version="1.0.0"
)

# Security middleware
app.add_middleware(IPWhitelistMiddleware)
app.add_middleware(APILoggingMiddleware)

# Define all allowed origins
origins = [
    "http://localhost:3000",
    "http://localhost:7001",
    "http://localhost:8000",
    "https://www.zgi.app",
    "https://zgi.app",
]

# Update CORS middleware with more specific configuration
app.add_middleware(
    CORSMiddleware,
    allow_origins=origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
    expose_headers=["*"],
    max_age=3600,  # Cache preflight requests for 1 hour
)

@app.exception_handler(RequestValidationError)
async def validation_exception_handler(request: Request, exc: RequestValidationError):
    """处理请求验证错误"""
    logger.error(f"Validation error: {exc.errors()}")
    return JSONResponse(
        status_code=422,
        content={
            "detail": "Invalid request",
            "errors": exc.errors()
        }
    )

@app.exception_handler(HTTPException)
async def http_exception_handler(request: Request, exc: HTTPException):
    """处理HTTP异常"""
    logger.error(f"HTTP error {exc.status_code}: {exc.detail}")
    return JSONResponse(
        status_code=exc.status_code,
        content={"detail": exc.detail}
    )

@app.exception_handler(SQLAlchemyError)
async def sqlalchemy_exception_handler(request: Request, exc: SQLAlchemyError):
    """处理数据库异常"""
    logger.error(f"Database error: {str(exc)}")
    logger.error(traceback.format_exc())
    return JSONResponse(
        status_code=500,
        content={"detail": "Database error occurred"}
    )

@app.middleware("http")
async def catch_exceptions_middleware(request: Request, call_next):
    """全局异常处理中间件"""
    try:
        response = await call_next(request)
        return response
    except Exception as exc:
        logger.error(f"Unhandled error: {str(exc)}")
        logger.error(traceback.format_exc())
        
        # 如果是已知的HTTP异常，保持原状态码
        if isinstance(exc, HTTPException):
            return JSONResponse(
                status_code=exc.status_code,
                content={"detail": exc.detail}
            )
        
        # 如果是数据库异常，返回500但不暴露具体错误
        if isinstance(exc, SQLAlchemyError):
            return JSONResponse(
                status_code=500,
                content={"detail": "Database error occurred"}
            )
        
        # 其他未知异常
        return JSONResponse(
            status_code=500,
            content={
                "detail": "Internal server error",
                "type": exc.__class__.__name__
            }
        )

# API routes
app.include_router(auth_router)
app.include_router(auth_console_router)
app.include_router(api_keys_router, prefix="/v1")
app.include_router(teams_router, prefix="/v1")
app.include_router(teams_console_router, prefix="/v1")
app.include_router(applications_console_router, prefix="/v1")
app.include_router(member_management_router, prefix="/v1")
app.include_router(usage_router, prefix="/v1")
app.include_router(users_router, prefix="/v1/console")

@app.get("/")
def root():
    """
    根路径，返回API基本信息
    """
    return {
        "message": "欢迎使用用户认证系统API",
        "version": "1.0.0",
        "documentation": "/docs"
    }