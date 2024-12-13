import os
from typing import AsyncGenerator
import pytest
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker

from app.db.base_class import Base
from app.features.chat.models.chat import ChatSession

# Use MySQL for testing - using the same database as development but with _test suffix
TEST_DATABASE_URL = "mysql+aiomysql://root@localhost:3306/zgi_test"

@pytest.fixture
async def engine():
    engine = create_async_engine(
        TEST_DATABASE_URL,
        echo=True,
        pool_pre_ping=True,
        pool_recycle=3600
    )
    
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
        await conn.run_sync(Base.metadata.create_all)
    
    try:
        yield engine
    finally:
        await engine.dispose()

@pytest.fixture
async def async_session(engine):
    async_session = sessionmaker(
        engine, class_=AsyncSession, expire_on_commit=False
    )
    async with async_session() as session:
        yield session

@pytest.fixture
def app():
    from app.features.chat.router.chat import router
    from fastapi import FastAPI
    app = FastAPI()
    app.include_router(router)
    return app

@pytest.fixture
async def client(app):
    from httpx import AsyncClient
    async with AsyncClient(app=app, base_url="http://test") as client:
        yield client
