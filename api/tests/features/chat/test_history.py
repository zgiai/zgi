import pytest
from datetime import datetime, timedelta
from httpx import AsyncClient
from fastapi import status
from app.models import ChatSession

@pytest.fixture
async def test_chat_sessions(test_db, test_user):
    """Create multiple test chat sessions"""
    sessions = []
    for i in range(15):  # Create 15 sessions
        session = ChatSession(
            user_id=test_user.id,
            model="gpt-4" if i % 2 == 0 else "gpt-3.5-turbo",
            title=f"Test Session {i}",
            messages=[
                {"role": "user", "content": f"Message {j}"} for j in range(3)
            ],
            created_at=datetime.utcnow() - timedelta(days=i)  # Different dates
        )
        test_db.add(session)
    
    test_db.commit()
    for session in sessions:
        test_db.refresh(session)
    
    return sessions

@pytest.mark.asyncio
async def test_get_chat_history(
    async_client: AsyncClient,
    test_user_headers,
    test_chat_sessions
):
    """Test getting chat history with pagination"""
    response = await async_client.get(
        "/v1/chat/history?page=1&page_size=5",
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["total"] == 15
    assert data["page"] == 1
    assert data["page_size"] == 5
    assert len(data["sessions"]) == 5

@pytest.mark.asyncio
async def test_get_chat_history_with_model_filter(
    async_client: AsyncClient,
    test_user_headers,
    test_chat_sessions
):
    """Test getting chat history filtered by model"""
    response = await async_client.get(
        "/v1/chat/history?model=gpt-4",
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert all(session["model"] == "gpt-4" for session in data["sessions"])

@pytest.mark.asyncio
async def test_get_chat_history_with_date_filter(
    async_client: AsyncClient,
    test_user_headers,
    test_chat_sessions
):
    """Test getting chat history filtered by date"""
    start_date = (datetime.utcnow() - timedelta(days=5)).isoformat()
    response = await async_client.get(
        f"/v1/chat/history?start_date={start_date}",
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert len(data["sessions"]) > 0
    assert all(
        datetime.fromisoformat(session["created_at"]) >= datetime.fromisoformat(start_date)
        for session in data["sessions"]
    )

@pytest.mark.asyncio
async def test_delete_chat_session(
    async_client: AsyncClient,
    test_user_headers,
    test_chat_sessions
):
    """Test deleting a chat session"""
    session_id = test_chat_sessions[0].id
    response = await async_client.delete(
        f"/v1/chat/sessions/{session_id}",
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_200_OK
    
    # Verify session is deleted
    response = await async_client.get(
        "/v1/chat/history",
        headers=test_user_headers
    )
    data = response.json()
    assert all(session["id"] != session_id for session in data["sessions"])

@pytest.mark.asyncio
async def test_delete_nonexistent_session(
    async_client: AsyncClient,
    test_user_headers
):
    """Test deleting a nonexistent chat session"""
    response = await async_client.delete(
        "/v1/chat/sessions/99999",
        headers=test_user_headers
    )
    
    assert response.status_code == status.HTTP_404_NOT_FOUND

@pytest.mark.asyncio
async def test_get_chat_history_unauthorized(async_client):
    """Test getting chat history without authentication"""
    response = await async_client.get("/v1/chat/history")
    assert response.status_code == status.HTTP_401_UNAUTHORIZED
