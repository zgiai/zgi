import pytest
from app.features.chat.service.chat import ChatService

async def test_create_chat_session(db, sample_chat_request):
    service = ChatService(db)
    chat_session = await service._create_chat_session(
        request=sample_chat_request,
        user_id=1,
        request_id="test-123",
        ip_address="127.0.0.1"
    )
    
    assert chat_session.user_id == 1
    assert chat_session.question == "Hello!"
    assert chat_session.model == "gpt-3.5-turbo"
    assert chat_session.status == 3

async def test_calculate_cost():
    service = ChatService(None)
    
    # Test GPT-4 cost calculation
    cost = service._calculate_cost("gpt-4", 100, 50)
    expected_cost = (100 * 0.03 + 50 * 0.06) / 1000
    assert cost == round(expected_cost, 7)
    
    # Test GPT-3.5-turbo cost calculation
    cost = service._calculate_cost("gpt-3.5-turbo", 100, 50)
    expected_cost = (100 * 0.001 + 50 * 0.002) / 1000
    assert cost == round(expected_cost, 7)
    
    # Test unknown model defaults to GPT-3.5-turbo pricing
    cost = service._calculate_cost("unknown-model", 100, 50)
    expected_cost = (100 * 0.001 + 50 * 0.002) / 1000
    assert cost == round(expected_cost, 7)

async def test_update_chat_session(db, sample_chat_session, sample_chat_response):
    service = ChatService(db)
    await service._update_chat_session(
        chat_session=sample_chat_session,
        response_data=sample_chat_response,
        answer="Hello! How can I help you today?"
    )
    
    assert sample_chat_session.status == 1
    assert sample_chat_session.answer == "Hello! How can I help you today!"
    assert sample_chat_session.prompt_tokens == 10
    assert sample_chat_session.completion_tokens == 8
    assert sample_chat_session.finish_reason == "stop"

async def test_get_chat_history(db, sample_chat_session):
    service = ChatService(db)
    history = await service.get_chat_history(user_id=1)
    
    assert len(history) == 1
    assert history[0].id == sample_chat_session.id
    assert history[0].conversation_id == "test-conv-123"

async def test_get_chat_detail(db, sample_chat_session):
    service = ChatService(db)
    detail = await service.get_chat_detail(
        chat_id=sample_chat_session.id,
        user_id=1
    )
    
    assert detail is not None
    assert detail.id == sample_chat_session.id
    assert detail.conversation_id == "test-conv-123"
    
    # Test non-existent chat
    detail = await service.get_chat_detail(chat_id=999, user_id=1)
    assert detail is None
