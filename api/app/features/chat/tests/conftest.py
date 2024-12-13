import pytest
from datetime import datetime
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool
from fastapi import FastAPI
from httpx import AsyncClient
from typing import AsyncGenerator

from app.db.base_class import Base
from app.features.chat.models import ChatSession
from app.features.chat.schemas.chat import ChatCompletionRequest, Message
from app.features.chat.router.chat import router
from app.main import app
from .test_config import async_session, engine

# Test database URL
TEST_DATABASE_URL = "sqlite+aiosqlite:///:memory:"

@pytest.fixture
def app() -> FastAPI:
    app = FastAPI()
    app.include_router(router)
    return app

@pytest.fixture
async def db() -> AsyncSession:
    engine = create_async_engine(
        TEST_DATABASE_URL,
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    
    async_session = sessionmaker(
        engine, class_=AsyncSession, expire_on_commit=False
    )
    
    async with async_session() as session:
        yield session
        
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)

@pytest.fixture
async def client(async_session: AsyncSession) -> AsyncGenerator:
    async with AsyncClient(app=app, base_url="http://test") as client:
        app.state.db = async_session
        yield client

@pytest.fixture
def auth_headers():
    return {"Authorization": "Bearer test-token"}

@pytest.fixture
def sample_chat_request():
    return {
        "model": "gpt-3.5-turbo",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello!"}
        ],
        "temperature": 0.7,
        "stream": False
    }

@pytest.fixture
def sample_chat_response():
    return {
        "id": "chatcmpl-123",
        "object": "chat.completion",
        "created": 1677652288,
        "model": "gpt-3.5-turbo",
        "choices": [{
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "Hello! How can I help you today?"
            },
            "finish_reason": "stop"
        }],
        "usage": {
            "prompt_tokens": 10,
            "completion_tokens": 8,
            "total_tokens": 18
        }
    }

@pytest.fixture
async def sample_chat_session(async_session: AsyncSession):
    from app.features.chat.models.chat import ChatSession
    
    chat_session = ChatSession(
        user_id=1,
        conversation_id="test-conv-123",
        request_id="test-req-123",
        question="Test question",
        answer="Test answer",
        model="gpt-3.5-turbo",
        prompt_tokens=10,
        completion_tokens=20,
        cost=0.001,
        ip_address="127.0.0.1"
    )
    
    async_session.add(chat_session)
    await async_session.commit()
    await async_session.refresh(chat_session)
    
    return chat_session

@pytest.fixture
def mock_openai_api(mocker):
    return mocker.patch("httpx.AsyncClient.post")

@pytest.fixture
def mock_current_user(mocker):
    mock_user = mocker.Mock()
    mock_user.id = 1
    mock_user.email = "test@example.com"
    
    mocker.patch(
        "app.core.security.get_current_user",
        return_value=mock_user
    )
    return mock_user
