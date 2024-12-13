import sys
import os

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from app.api.v1.router import router as api_v1_router
from dotenv import load_dotenv
import logging
from app.core.config import settings
from app.services.rag_service import rag_service
from app.utils.mysql_manager import mysql_manager

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Load environment variables
load_dotenv()

# Get OPENAI_API_KEY from environment
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")
if not OPENAI_API_KEY:
    logger.error("OPENAI_API_KEY is not set in the environment")
    raise ValueError("OPENAI_API_KEY is required")

logger.info(f"OPENAI_API_KEY is {'set' if OPENAI_API_KEY else 'not set'}")

# Create FastAPI app
app = FastAPI(
    title=settings.PROJECT_NAME,
    openapi_url=f"{settings.API_V1_STR}/openapi.json",
    version="1.0.0"
)

# Configure CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000"],  # 允许本地前端访问
    allow_credentials=True,
    allow_methods=["*"],  # 允许所有方法
    allow_headers=["*"],  # 允许所有头
)

# Include routers
app.include_router(api_v1_router, prefix="/v1")

@app.on_event("startup")
async def startup_event():
    """Initialize services on startup"""
    try:
        # Initialize database connection
        mysql_manager.connect()
        logger.info("Successfully connected to database")
    except Exception as e:
        logger.error(f"Error during startup: {str(e)}")
        raise

@app.on_event("shutdown")
async def shutdown_event():
    """Cleanup on shutdown"""
    try:
        # Close database connection
        mysql_manager.disconnect()
        await rag_service.close()
        logger.info("Successfully disconnected from database")
    except Exception as e:
        logger.error(f"Error during shutdown: {str(e)}")

if __name__ == "__main__":
    import uvicorn
    
    port = 7001  # Always use port 7001 as per requirements
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=port,
        reload=True
    )
