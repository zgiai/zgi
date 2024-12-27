from fastapi import FastAPI, Request, HTTPException, status
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError
from sqlalchemy.exc import SQLAlchemyError
import traceback
import logging
import uvicorn
from app.core.logging.api_logger import APILoggingMiddleware
from app.core.database import Base, engine
from app.core.error_handlers import setup_error_handlers
from app.core.config import settings

from app.features.auth.router import router as auth_router
from app.features.organizations.router import router as org_router
from app.features.users.router import router as users_router
from app.features.projects.router import router as projects_router
from app.features.usage.router import router as usage_router
from app.features.applications.console.router import router as applications_router
from app.features.api_keys.router import router as api_keys_router
from app.features.providers.router.provider import router as providers_router
from app.features.providers.router.model import router as models_router
from app.features.gateway.router import router as gateway_router

# Configure logging
logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)

# Create database tables
# Base.metadata.create_all(bind=engine)
# async def create_tables():
#     async with engine.begin() as conn:
#         await conn.run_sync(Base.metadata.create_all)
#
# asyncio.run(create_tables())

def handle_http_exception(req: Request, exc: Exception) -> JSONResponse:
    if isinstance(exc, HTTPException):
        msg = {
            'status_code': exc.status_code,
            'status_message': exc.detail['error'] if isinstance(exc.detail, dict) else exc.detail
        }
    else:
        msg = {'status_code': 500, 'status_message': str(exc)}
    logger.error(f'{req.method} {req.url} {str(exc)}')
    return JSONResponse(content=msg)
    # return ORJSONResponse(content=msg)


def handle_request_validation_error(req: Request, exc: RequestValidationError) -> JSONResponse:
    msg = {'status_code': status.HTTP_422_UNPROCESSABLE_ENTITY, 'status_message': exc.errors()}
    logger.error(f'{req.method} {req.url} {exc.errors()} {exc.body}')
    return JSONResponse(content=msg)
    # return ORJSONResponse(content=msg)


_EXCEPTION_HANDLERS = {
    HTTPException: handle_http_exception,
    RequestValidationError: handle_request_validation_error,
    Exception: handle_http_exception
}

# Create FastAPI app
app = FastAPI(
    title="ZGI API",
    description="ZGI API Documentation",
    version="1.0.0",
    exception_handlers=_EXCEPTION_HANDLERS,
    debug=True  # Enable debug mode for detailed error messages
)

# Setup error handlers
setup_error_handlers(app)

# Add middleware
app.add_middleware(APILoggingMiddleware)

# Define all allowed origins
origins = [
    "http://localhost:7001",
    "http://localhost:3000",
    "https://www.zgi.app",
    "https://zgi.app",
]

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

@app.on_event("startup")
async def startup():
    # Create tables
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

@app.on_event("shutdown")
async def shutdown():
    await engine.dispose()

@app.exception_handler(RequestValidationError)
async def validation_exception_handler(request: Request, exc: RequestValidationError):
    """Handle request validation errors"""
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
    """Global exception handling middleware"""
    try:
        response = await call_next(request)
        return response
    except Exception as exc:
        logger.error(f"Unhandled error: {str(exc)}")
        logger.error(traceback.format_exc())
        
        # If it's a known HTTP exception, keep the original status code
        if isinstance(exc, HTTPException):
            return JSONResponse(
                status_code=exc.status_code,
                content={"detail": exc.detail}
            )
        
        # Other unknown exceptions
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
app.include_router(api_keys_router, prefix="/v1/api-keys", tags=["API Keys"])
app.include_router(providers_router)
app.include_router(models_router)
app.include_router(gateway_router, tags=["LLM Gateway"])

@app.get("/")
def root():
    """
    Root path, returns basic API information
    """
    return {
        "name": "ZGI API",
        "version": "1.0.0",
        "status": "ok"
    }

if __name__ == "__main__":
    uvicorn.run(
        "app.main:app",
        host=settings.HOST,
        port=settings.PORT,
        reload=True
    )