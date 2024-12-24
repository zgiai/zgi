from sqlalchemy import create_engine
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import declarative_base, declared_attr, DeclarativeBase
from sqlalchemy.orm import sessionmaker
from sqlalchemy.exc import SQLAlchemyError
from fastapi import HTTPException
import logging
import os
import aiomysql
from typing import Any, Dict

from app.core.config import settings

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Log database connection details
logger.info(f"Connecting to database: {settings.DB_HOST}:{settings.DB_PORT}/{settings.DB_DATABASE}")

SQLALCHEMY_DATABASE_URL = settings.SQLALCHEMY_DATABASE_URL

engine = create_async_engine(
    SQLALCHEMY_DATABASE_URL,
    echo=True,
    pool_pre_ping=True,
    pool_recycle=3600,
    pool_size=5,
    max_overflow=10
)

sync_engine = create_engine(
    SQLALCHEMY_DATABASE_URL.replace("+aiomysql", "+pymysql"),
    echo=True,
    pool_pre_ping=True,
    pool_recycle=3600,
    pool_size=5,
    max_overflow=10
)

SessionLocal = sessionmaker(
    engine,
    class_=AsyncSession,
    expire_on_commit=False,
    autocommit=False,
    autoflush=False
)

SyncSessionLocal = sessionmaker(
    sync_engine,
    expire_on_commit=False,
    autocommit=False,
    autoflush=False
)

class Base:
    """
    Base class for all database models with common functionality.
    """
    
    @declared_attr
    def __tablename__(cls) -> str:
        """Generate table name automatically from class name"""
        return cls.__name__.lower()

    @declared_attr
    def __table_args__(cls) -> Dict[str, str]:
        """Set MySQL InnoDB as the default engine"""
        return {'mysql_engine': 'InnoDB', 'extend_existing': True}

    def to_dict(self) -> Dict[str, Any]:
        """Convert model instance to dictionary"""
        return {c.name: getattr(self, c.name) for c in self.__table__.columns}

    def __repr__(self) -> str:
        """String representation of the model"""
        attrs = ', '.join(f'{c.name}={getattr(self, c.name)}' 
                         for c in self.__table__.columns)
        return f"{self.__class__.__name__}({attrs})"

# Create declarative base
Base = declarative_base(cls=Base)

async def get_db():
    """Dependency for database session management"""
    async with SessionLocal() as session:
        try:
            yield session
            await session.commit()
        except Exception as e:
            await session.rollback()
            raise
        finally:
            await session.close()

def get_sync_db():
    """Dependency for synchronous database session management"""
    with SyncSessionLocal() as session:
        try:
            yield session
            session.commit()
        except Exception as e:
            session.rollback()
            raise
        finally:
            session.close()

async def handle_db_operation(operation):
    """
    Wrapper for database operations with error handling
    
    Args:
        operation: Async callable that performs database operation
        
    Returns:
        Result of the database operation
        
    Raises:
        HTTPException: On database operation failure
    """
    try:
        return await operation
    except SQLAlchemyError as e:
        logger.error(f"Database error: {str(e)}")
        raise HTTPException(status_code=500, detail="Database operation failed")
    except Exception as e:
        logger.error(f"Unexpected error: {str(e)}")
        raise HTTPException(status_code=500, detail="Internal server error")

# Import all models here so that they are registered with SQLAlchemy
from app.features.chat.models import ChatSession, ChatFile, Conversation, ChatMessage
from app.features.providers.models import Category, Model, Provider
from app.features.gateway.models import APIKey, UserQuota
from app.models.knowledge_base import KnowledgeBase

# Add all models that should be included in migrations
__all__ = [
    "Base",
    "get_db",
    "get_sync_db",
    "handle_db_operation",
    "ChatSession",
    "ChatFile",
    "Conversation",
    "ChatMessage",
    "Category",
    "Model",
    "Provider",
    "APIKey",
    "UserQuota",
    "KnowledgeBase"
]
