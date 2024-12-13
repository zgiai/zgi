import pytest
from fastapi import FastAPI
from httpx import AsyncClient
from app.features.chat.router.chat import router
from app.features.chat.schemas.chat import ChatCompletionRequest, Message
from app.features.chat.models.chat import ChatSession

app = FastAPI()
app.include_router(router)

@pytest.fixture
def auth_headers():
    return {"Authorization": "Bearer test-token"}

@pytest.fixture
def client():
    return AsyncClient(app=app, base_url="http://test")

async def test_create_chat_completion(async_session, auth_headers):
    request_data = {
        "model": "gpt-3.5-turbo",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello!"}
        ],
        "temperature": 0.7,
        "stream": False
    }
    
    async with client as client:
        response = await client.post(
            "/v1/chat/completions",
            json=request_data,
            headers=auth_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert "id" in data
        assert data["object"] == "chat.completion"
        assert "choices" in data
        assert len(data["choices"]) > 0
        assert "message" in data["choices"][0]
        assert "content" in data["choices"][0]["message"]

async def test_create_chat_completion_streaming(async_session, auth_headers):
    request_data = {
        "model": "gpt-3.5-turbo",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello!"}
        ],
        "temperature": 0.7,
        "stream": True
    }
    
    async with client as client:
        response = await client.post(
            "/v1/chat/completions",
            json=request_data,
            headers=auth_headers
        )
        
        assert response.status_code == 200
        assert response.headers["content-type"] == "text/event-stream"

async def test_get_chat_history(async_session, auth_headers):
    # First create some chat sessions
    chat_sessions = [
        ChatSession(
            user_id=1,
            conversation_id=f"test-conv-{i}",
            request_id=f"test-req-{i}",
            question=f"Test question {i}",
            answer=f"Test answer {i}",
            model="gpt-3.5-turbo",
            prompt_tokens=10,
            completion_tokens=20,
            cost=0.001,
            ip_address="127.0.0.1"
        ) for i in range(3)
    ]
    
    for session in chat_sessions:
        async_session.add(session)
    await async_session.commit()
    
    async with client as client:
        response = await client.get(
            "/v1/chat/history",
            headers=auth_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert "total" in data
        assert "items" in data
        assert len(data["items"]) == 3

async def test_get_chat_history_with_conversation_id(async_session, auth_headers):
    # Create chat sessions with specific conversation_id
    conversation_id = "test-conv-specific"
    chat_sessions = [
        ChatSession(
            user_id=1,
            conversation_id=conversation_id,
            request_id=f"test-req-{i}",
            question=f"Test question {i}",
            answer=f"Test answer {i}",
            model="gpt-3.5-turbo",
            prompt_tokens=10,
            completion_tokens=20,
            cost=0.001,
            ip_address="127.0.0.1"
        ) for i in range(2)
    ]
    
    for session in chat_sessions:
        async_session.add(session)
    await async_session.commit()
    
    async with client as client:
        response = await client.get(
            f"/v1/chat/history?conversation_id={conversation_id}",
            headers=auth_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 2
        assert all(item["conversation_id"] == conversation_id for item in data["items"])

async def test_get_chat_detail(async_session, auth_headers):
    # Create a chat session
    chat_session = ChatSession(
        user_id=1,
        conversation_id="test-conv-detail",
        request_id="test-req-detail",
        question="Test detail question",
        answer="Test detail answer",
        model="gpt-3.5-turbo",
        prompt_tokens=10,
        completion_tokens=20,
        cost=0.001,
        ip_address="127.0.0.1"
    )
    
    async_session.add(chat_session)
    await async_session.commit()
    await async_session.refresh(chat_session)
    
    async with client as client:
        response = await client.get(
            f"/v1/chat/detail/{chat_session.id}",
            headers=auth_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert data["id"] == chat_session.id
        assert data["question"] == "Test detail question"
        assert data["answer"] == "Test detail answer"

async def test_unauthorized_access(async_session):
    endpoints = [
        "/v1/chat/completions",
        "/v1/chat/history",
        "/v1/chat/detail/1"
    ]
    
    async with client as client:
        for endpoint in endpoints:
            if endpoint == "/v1/chat/completions":
                response = await client.post(endpoint, json={})
            else:
                response = await client.get(endpoint)
            
            assert response.status_code == 401
