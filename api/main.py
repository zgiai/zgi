import sys
import os

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from app.api.v1.router import router as api_v1_router
from app.utils.weaviate_manager import WeaviateManager
from dotenv import load_dotenv
from wasabi import msg
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

app = FastAPI(title=settings.PROJECT_NAME)

# 添加 CORS 中间件配置
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # 允许本地前端访问
    allow_credentials=True,
    allow_methods=["*"],  # 允许所有方法
    allow_headers=["*"],  # 允许所有头
)

app.include_router(api_v1_router, prefix="/v1")

weaviate_manager = WeaviateManager()

@app.on_event("startup")
async def startup_event():
    try:
        weaviate_manager.connect()
        mysql_manager.initialize_pool()
        mysql_manager.connect()
        logger.info("Successfully connected to Weaviate")
    except Exception as e:
        logger.error(f"Failed to connect to Weaviate: {str(e)}")
        raise

@app.on_event("shutdown")
async def shutdown_event():
    try:
        weaviate_manager.close()
        await rag_service.close()
        mysql_manager.disconnect()
        logger.info("Successfully closed all connections")
    except Exception as e:
        logger.error(f"Failed to close connections: {str(e)}") 

if __name__ == "__main__":
    import uvicorn
    
    port = 8088 if os.environ.get("ENVIRONMENT") == "production" else 7001
    
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=port,
        reload=True
    )
