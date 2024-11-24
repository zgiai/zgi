import sys
import os
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from app.api.v1.router import router as api_v1_router  # Fixed import path
from app.utils.weaviate_manager import WeaviateManager  # Fixed import path
from dotenv import load_dotenv
import logging
from app.core.config import settings  # Fixed import path
from app.services.rag_service import rag_service  # Fixed import path
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

app = FastAPI(title=settings.PROJECT_NAME)

# Define all allowed origins
origins = [
    "http://localhost:3000",
    "http://localhost:8000",
    "https://www.zgi.app",
    "https://zgi.app",
    # 添加其他需要的域名
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