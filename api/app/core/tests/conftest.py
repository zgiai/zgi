import os
from typing import AsyncGenerator
import pytest
import asyncio
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker

from app.core.database import Base  
from app.core.config import settings

# Use the same database as development
TEST_DATABASE_URL = settings.SQLALCHEMY_DATABASE_URL

@pytest.fixture(scope="function")
def event_loop():
    """Create an instance of the default event loop for each test case."""
    policy = asyncio.get_event_loop_policy()
    loop = policy.new_event_loop()
    yield loop
    loop.close()

@pytest.fixture(scope="function")
async def engine():
    """Create test database engine"""
    engine = create_async_engine(
        TEST_DATABASE_URL,
        echo=True,
        pool_pre_ping=True,
        pool_recycle=3600
    )
    
    # Create all tables
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    
    try:
        yield engine
    finally:
        await engine.dispose()

@pytest.fixture(scope="function")
async def async_session(engine) -> AsyncGenerator[AsyncSession, None]:
    """Create test database session"""
    async_session_maker = sessionmaker(
        engine, 
        class_=AsyncSession, 
        expire_on_commit=False,
        autocommit=False,
        autoflush=False
    )
    
    async with async_session_maker() as session:
        try:
            yield session
            await session.commit()
        except Exception:
            await session.rollback()
            raise
        finally:
            await session.close()

@pytest.fixture(scope="function")
def app():
    """Create test FastAPI application"""
    from app.main import app
    return app

@pytest.fixture(scope="function")
async def client(app):
    """Create test HTTP client"""
    from httpx import AsyncClient
    async with AsyncClient(app=app, base_url="http://test") as client:
        yield client
