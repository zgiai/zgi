from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import declarative_base
from sqlalchemy.orm import sessionmaker
from sqlalchemy.exc import SQLAlchemyError
from fastapi import HTTPException
import logging
import os
import aiomysql

from app.core.config import settings

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Log database connection details
logger.info(f"Connecting to database: {settings.DB_HOST}:{settings.DB_PORT}/{settings.DB_DATABASE}")
logger.info(f"Database URL: {settings.SQLALCHEMY_DATABASE_URL}")

SQLALCHEMY_DATABASE_URL = settings.SQLALCHEMY_DATABASE_URL

engine = create_async_engine(
    SQLALCHEMY_DATABASE_URL,
    echo=True,
    pool_pre_ping=True,  # Enable connection health checks
    pool_recycle=3600,   # Recycle connections after 1 hour
    pool_size=5,         # Set a reasonable pool size
    max_overflow=10      # Allow up to 10 additional connections
)

SessionLocal = sessionmaker(
    engine,
    class_=AsyncSession,
    expire_on_commit=False,
    autocommit=False,
    autoflush=False
)

Base = declarative_base()

async def get_db():
    async with SessionLocal() as session:
        try:
            yield session
        except SQLAlchemyError as e:
            await session.rollback()
            logger.error(f"Database error: {str(e)}")
            raise
        finally:
            await session.close()

async def handle_db_operation(operation):
    """
    Wrapper for database operations with detailed error handling
    
    Usage:
        result = await handle_db_operation(lambda: db.query(User).filter(User.id == user_id).first())
    """
    try:
        return await operation()
    except SQLAlchemyError as e:
        logger.error(f"Database operation error: {str(e)}")
        raise HTTPException(status_code=500, detail="Database operation failed")
