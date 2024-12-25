"""Database configuration and session management."""

from sqlalchemy import create_engine
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import declarative_base, declared_attr, DeclarativeBase, sessionmaker
from sqlalchemy.orm import sessionmaker
from sqlalchemy.exc import SQLAlchemyError
from fastapi import HTTPException
import logging
import os
from typing import Any, Dict, Generator, AsyncGenerator

from app.core.config import settings

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Log database connection details
logger.info(f"Connecting to database: {settings.MYSQL_SERVER}:{settings.MYSQL_PORT}/{settings.MYSQL_DB}")

# Create async engine for FastAPI
engine = create_async_engine(
    settings.SQLALCHEMY_DATABASE_URL,
    pool_size=20,
    max_overflow=0,
    pool_timeout=30,
    pool_recycle=1800,
    echo=settings.SQL_ECHO
)

# Create sync database URL for testing and CLI tools
SYNC_DATABASE_URL = settings.SQLALCHEMY_DATABASE_URL.replace("+aiomysql", "+pymysql")
sync_engine = create_engine(
    SYNC_DATABASE_URL,
    pool_size=20,
    max_overflow=0,
    pool_timeout=30,
    pool_recycle=1800,
    echo=settings.SQL_ECHO
)

# Create session factories
AsyncSessionLocal = sessionmaker(
    class_=AsyncSession,
    autocommit=False,
    autoflush=False,
    bind=engine,
    expire_on_commit=False
)

SyncSessionLocal = sessionmaker(
    autocommit=False,
    autoflush=False,
    bind=sync_engine,
    expire_on_commit=False
)

class Base:
    """Base class for all database models with common functionality."""
    
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
        return f"<{self.__class__.__name__}(id={getattr(self, 'id', None)})>"

# Create declarative base
Base = declarative_base(cls=Base)

async def get_db() -> AsyncGenerator:
    """Get asynchronous database session"""
    async with AsyncSessionLocal() as session:
        try:
            yield session
        finally:
            await session.close()

def get_sync_db() -> Generator:
    """Get synchronous database session"""
    db = SyncSessionLocal()
    try:
        yield db
    finally:
        db.close()

async def init_db():
    """Initialize database"""
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

def handle_db_operation(operation):
    """Wrapper for database operations with error handling"""
    try:
        return operation()
    except SQLAlchemyError as e:
        logger.error(f"Database error: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Database error: {str(e)}"
        ) from e
