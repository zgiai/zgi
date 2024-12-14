import pytest
from app.features.chat.service.chat_service import ChatService
from app.features.chat.models.chat import ChatSession, ChatFile
from app.features.providers.models.provider import ModelProvider
from app.features.providers.models.model import ProviderModel
from datetime import datetime

@pytest.fixture
async def chat_service(async_session):
    return ChatService(async_session)

@pytest.fixture
async def model_provider(async_session):
    provider = ModelProvider(
        provider_name="test_provider",
        enabled=True,
        api_key="test_key",
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    async_session.add(provider)
    await async_session.commit()
    await async_session.refresh(provider)
    return provider

@pytest.fixture
async def provider_model(async_session, model_provider):
    model = ProviderModel(
        provider_id=model_provider.id,
        model_name="test_model",
        type="chat",
        supports_streaming=True,
        supports_function_calling=True,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    async_session.add(model)
    await async_session.commit()
    await async_session.refresh(model)
    return model

@pytest.fixture
async def chat_session(async_session, provider_model):
    session = ChatSession(
        model_id=provider_model.id,
        user_id=1,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    async_session.add(session)
    await async_session.commit()
    await async_session.refresh(session)
    return session

@pytest.mark.asyncio
async def test_create_chat_session(chat_service, provider_model):
    """Test creating a new chat session"""
    session = await chat_service.create_session(
        user_id=1,
        model_id=provider_model.id
    )
    assert session.id is not None
    assert session.user_id == 1
    assert session.model_id == provider_model.id
    assert session.created_at is not None
    assert session.updated_at is not None
    assert session.deleted_at is None

@pytest.mark.asyncio
async def test_get_chat_session(chat_service, chat_session):
    """Test retrieving a chat session"""
    session = await chat_service.get_session(chat_session.id)
    assert session.id == chat_session.id
    assert session.user_id == chat_session.user_id
    assert session.model_id == chat_session.model_id

@pytest.mark.asyncio
async def test_list_user_sessions(chat_service, chat_session):
    """Test listing chat sessions for a user"""
    sessions = await chat_service.list_user_sessions(user_id=1)
    assert len(sessions) > 0
    assert any(s.id == chat_session.id for s in sessions)

@pytest.mark.asyncio
async def test_delete_chat_session(chat_service, chat_session):
    """Test soft deleting a chat session"""
    await chat_service.delete_session(chat_session.id)
    session = await chat_service.get_session(chat_session.id)
    assert session.deleted_at is not None

@pytest.mark.asyncio
async def test_upload_chat_file(chat_service, chat_session):
    """Test uploading a file to a chat session"""
    file = await chat_service.upload_file(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt"
    )
    assert file.id is not None
    assert file.session_id == chat_session.id
    assert file.filename == "test.txt"
    assert file.content_type == "text/plain"
    assert file.file_size == 100
    assert file.file_path == "/path/to/test.txt"

@pytest.mark.asyncio
async def test_get_chat_file(chat_service, chat_session):
    """Test retrieving a chat file"""
    # First upload a file
    file = await chat_service.upload_file(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt"
    )
    
    # Then retrieve it
    retrieved_file = await chat_service.get_file(file.id)
    assert retrieved_file.id == file.id
    assert retrieved_file.session_id == chat_session.id
    assert retrieved_file.filename == "test.txt"

@pytest.mark.asyncio
async def test_list_session_files(chat_service, chat_session):
    """Test listing files in a chat session"""
    # First upload a file
    await chat_service.upload_file(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt"
    )
    
    # Then list files
    files = await chat_service.list_session_files(chat_session.id)
    assert len(files) > 0
    assert all(f.session_id == chat_session.id for f in files)

@pytest.mark.asyncio
async def test_delete_chat_file(chat_service, chat_session):
    """Test deleting a chat file"""
    # First upload a file
    file = await chat_service.upload_file(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt"
    )
    
    # Then delete it
    await chat_service.delete_file(file.id)
    
    # Try to retrieve it - should be None or raise exception
    deleted_file = await chat_service.get_file(file.id)
    assert deleted_file is None or deleted_file.deleted_at is not None

@pytest.mark.asyncio
async def test_get_nonexistent_session(chat_service):
    """Test retrieving a non-existent chat session"""
    session = await chat_service.get_session(999)
    assert session is None

@pytest.mark.asyncio
async def test_get_deleted_session(chat_service, chat_session):
    """Test retrieving a deleted chat session"""
    await chat_service.delete_session(chat_session.id)
    session = await chat_service.get_session(chat_session.id)
    assert session.deleted_at is not None

@pytest.mark.asyncio
async def test_list_empty_user_sessions(chat_service):
    """Test listing chat sessions for a user with no sessions"""
    sessions = await chat_service.list_user_sessions(user_id=999)
    assert len(sessions) == 0

@pytest.mark.asyncio
async def test_get_nonexistent_file(chat_service):
    """Test retrieving a non-existent chat file"""
    file = await chat_service.get_file(999)
    assert file is None

@pytest.mark.asyncio
async def test_list_empty_session_files(chat_service, chat_session):
    """Test listing files for a session with no files"""
    files = await chat_service.list_session_files(chat_session.id)
    assert len(files) == 0

@pytest.mark.asyncio
async def test_upload_file_invalid_session(chat_service):
    """Test uploading a file to a non-existent session"""
    with pytest.raises(Exception):  # Adjust exception type as needed
        await chat_service.upload_file(
            session_id=999,
            filename="test.txt",
            content_type="text/plain",
            file_size=100,
            file_path="/path/to/test.txt"
        )
