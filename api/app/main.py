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
from app.core.error_handlers import setup_error_handlers

from app.features.auth.router import router as auth_router
from app.features.organizations.router import router as org_router
from app.features.users.router import router as users_router
from app.features.projects.router import router as projects_router
from app.features.usage.router import router as usage_router
from app.features.applications.console.router import router as applications_router

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Create database tables
Base.metadata.create_all(bind=engine)

# Create FastAPI app
app = FastAPI(
    title="ZGI API",
    description="ZGI API Documentation",
    version="1.0.0",
    debug=True  # Enable debug mode for detailed error messages
)

# Setup error handlers
setup_error_handlers(app)

# Security middleware
app.add_middleware(IPWhitelistMiddleware)
app.add_middleware(APILoggingMiddleware)

# Define all allowed origins
origins = [
    "http://localhost:7001",
    "https://www.zgi.app",
    "https://zgi.app",
]

# Update CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
    expose_headers=["*"],
    max_age=3600,
)

@app.exception_handler(RequestValidationError)
async def validation_exception_handler(request: Request, exc: RequestValidationError):
    """处理请求验证错误"""
    logger.error(f"Validation error: {exc.errors()}")
    return JSONResponse(
        status_code=422,
        content={
            "detail": {
                "message": "Invalid request",
                "errors": exc.errors()
            }
        }
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
        
        # 其他未知异常
        return JSONResponse(
            status_code=500,
            content={
                "detail": {
                    "message": "Internal server error",
                    "error_type": exc.__class__.__name__,
                    "error_details": str(exc)
                }
            }
        )

# API routers
app.include_router(auth_router)
app.include_router(users_router, prefix="/v1/users", tags=["Users"])
app.include_router(org_router, prefix="/v1/organizations", tags=["Organizations"])
app.include_router(projects_router, prefix="/v1/projects", tags=["Projects"])
app.include_router(usage_router, prefix="/v1/usage", tags=["Usage"])
app.include_router(applications_router, prefix="/v1/applications", tags=["Applications"])

@app.get("/")
async def root():
    """
    根路径，返回API基本信息
    """
    return {
        "name": "ZGI API",
        "version": "1.0.0",
        "description": "ZGI API Documentation"
    }