import pytest
from httpx import AsyncClient
from fastapi import status
from app.features.chat.schemas import ChatMessage
from app.models import ChatSession

@pytest.fixture
async def test_chat_session(test_db, test_user):
    """Create a test chat session"""
    session = ChatSession(
        user_id=test_user.id,
        model="gpt-4",
        title="Test Session",
        messages=[
            {"role": "user", "content": "Hello"},
            {"role": "assistant", "content": "Hi there!"}
        ]
    )
    test_db.add(session)
    test_db.commit()
    test_db.refresh(session)
    return session

@pytest.mark.asyncio
async def test_switch_model(
    async_client: AsyncClient,
    test_user_headers,
    test_chat_session
):
    """Test switching model in a chat session"""
    request_data = {
        "session_id": test_chat_session.id,
        "model": "gpt-3.5-turbo"
    }
    
    response = await async_client.post(
        "/v1/chat/switch-model",
        json=request_data,
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["session_id"] == test_chat_session.id
    assert data["model"] == "gpt-3.5-turbo"
    assert len(data["messages"]) == 2
    assert data["messages"][0]["content"] == "Hello"
    assert data["messages"][1]["content"] == "Hi there!"

@pytest.mark.asyncio
async def test_switch_model_invalid_session(
    async_client: AsyncClient,
    test_user_headers
):
    """Test switching model with invalid session ID"""
    request_data = {
        "session_id": 999999,  # Non-existent session
        "model": "gpt-3.5-turbo"
    }
    
    response = await async_client.post(
        "/v1/chat/switch-model",
        json=request_data,
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_404_NOT_FOUND

@pytest.mark.asyncio
async def test_switch_model_unauthorized(
    async_client: AsyncClient,
    test_chat_session
):
    """Test switching model without authentication"""
    request_data = {
        "session_id": test_chat_session.id,
        "model": "gpt-3.5-turbo"
    }
    
    response = await async_client.post(
        "/v1/chat/switch-model",
        json=request_data
    )
    
    assert response.status_code == status.HTTP_401_UNAUTHORIZED

@pytest.mark.asyncio
async def test_chat_with_session(
    async_client: AsyncClient,
    test_user_headers,
    test_chat_session
):
    """Test chat with existing session"""
    request_data = {
        "model": "gpt-4",
        "messages": [
            {
                "role": "user",
                "content": "How are you?"
            }
        ],
        "session_id": test_chat_session.id
    }
    
    async with async_client.stream(
        "POST",
        "/v1/chat/stream",
        json=request_data,
        headers=test_user_headers
    ) as response:
        assert response.status_code == status.HTTP_200_OK
        assert response.headers["content-type"] == "text/event-stream"
