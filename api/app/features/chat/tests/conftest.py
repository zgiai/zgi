import pytest
from datetime import datetime
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool
from fastapi import FastAPI
from httpx import AsyncClient
from typing import AsyncGenerator

from app.core.database import Base  
from app.features.chat.models import ChatSession
from app.features.chat.schemas.chat import ChatCompletionRequest, Message
from app.features.chat.router.chat import router
from app.main import app
from .test_config import async_session, engine

# Test database URL
TEST_DATABASE_URL = "sqlite+aiosqlite:///:memory:"

@pytest.fixture
def app() -> FastAPI:
    """Create test FastAPI application"""
    from app.main import app
    return app

@pytest.fixture
async def db(engine):
    """Set up and tear down test database"""
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
        await conn.run_sync(Base.metadata.create_all)
    
    yield
    
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)

@pytest.fixture
async def client(app):
    """Create test HTTP client"""
    async with AsyncClient(app=app, base_url="http://test") as client:
        yield client

@pytest.fixture
def auth_headers():
    """Create test authentication headers"""
    return {"Authorization": "Bearer test_token"}

@pytest.fixture
def sample_chat_request():
    """Create sample chat completion request"""
    return ChatCompletionRequest(
        model="gpt-3.5-turbo",
        messages=[
            Message(role="system", content="You are a helpful assistant."),
            Message(role="user", content="Hello!")
        ],
        stream=False
    )

@pytest.fixture
def sample_chat_response():
    """Create sample chat completion response"""
    return {
        "id": "chatcmpl-123",
        "object": "chat.completion",
        "created": int(datetime.now().timestamp()),
        "model": "gpt-3.5-turbo-0613",
        "choices": [{
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "Hello! How can I assist you today?"
            },
            "finish_reason": "stop"
        }],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 10,
            "total_tokens": 20
        }
    }

@pytest.fixture
async def sample_chat_session(async_session: AsyncSession):
    """Create sample chat session in database"""
    session = ChatSession(
        user_id=1,
        conversation_id="test-conv-123",
        request_id="test-req-123",
        question="Hello!",
        answer="Hi there!",
        model="gpt-3.5-turbo",
        prompt_tokens=10,
        completion_tokens=10,
        total_tokens=21,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    async_session.add(session)
    await async_session.commit()
    await async_session.refresh(session)
    return session

@pytest.fixture
def mock_openai_api(mocker):
    """Mock OpenAI API responses"""
    return mocker.patch("app.features.chat.service.chat.openai.ChatCompletion.acreate")

@pytest.fixture
def mock_current_user(mocker):
    """Mock current user for authentication"""
    return mocker.patch(
        "app.features.chat.router.chat.get_current_user",
        return_value={
            "id": 1,
            "email": "test@example.com",
            "is_active": True
        }
    )
