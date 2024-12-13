import pytest
from app.features.chat.service.chat_service import ChatService
from app.features.chat.models.chat import ChatSession, ChatFile
from app.features.providers.models.provider import ModelProvider
from app.features.providers.models.model import ProviderModel
from datetime import datetime

@pytest.fixture
def chat_service(db):
    return ChatService(db)

@pytest.fixture
def model_provider(db):
    provider = ModelProvider(
        provider_name="test_provider",
        enabled=True,
        api_key="test_key",
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    db.add(provider)
    db.commit()
    return provider

@pytest.fixture
def provider_model(db, model_provider):
    model = ProviderModel(
        provider_id=model_provider.id,
        model_name="test_model",
        type="chat",
        supports_streaming=True,
        supports_function_calling=True,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    db.add(model)
    db.commit()
    return model

@pytest.fixture
def chat_session(db, provider_model):
    session = ChatSession(
        model_id=provider_model.id,
        user_id=1,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    db.add(session)
    db.commit()
    return session

def test_create_chat_session(chat_service, provider_model):
    """Test creating a new chat session"""
    session = chat_service.create_session(
        user_id=1,
        model_id=provider_model.id
    )
    assert session.id is not None
    assert session.user_id == 1
    assert session.model_id == provider_model.id
    assert session.created_at is not None
    assert session.updated_at is not None
    assert session.deleted_at is None

def test_get_chat_session(chat_service, chat_session):
    """Test retrieving a chat session"""
    session = chat_service.get_session(chat_session.id)
    assert session.id == chat_session.id
    assert session.user_id == chat_session.user_id
    assert session.model_id == chat_session.model_id

def test_list_user_sessions(chat_service, chat_session):
    """Test listing chat sessions for a user"""
    sessions = chat_service.list_user_sessions(user_id=1)
    assert len(sessions) > 0
    assert any(s.id == chat_session.id for s in sessions)

def test_delete_chat_session(chat_service, chat_session):
    """Test soft deleting a chat session"""
    chat_service.delete_session(chat_session.id)
    session = chat_service.get_session(chat_session.id)
    assert session.deleted_at is not None

def test_upload_chat_file(chat_service, chat_session):
    """Test uploading a file to a chat session"""
    file = chat_service.upload_file(
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

def test_get_chat_file(chat_service, chat_session):
    """Test retrieving a chat file"""
    file = ChatFile(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt",
        created_at=datetime.utcnow()
    )
    chat_service.db.add(file)
    chat_service.db.commit()

    retrieved_file = chat_service.get_file(file.id)
    assert retrieved_file.id == file.id
    assert retrieved_file.session_id == chat_session.id
    assert retrieved_file.filename == "test.txt"

def test_list_session_files(chat_service, chat_session):
    """Test listing files in a chat session"""
    file = ChatFile(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt",
        created_at=datetime.utcnow()
    )
    chat_service.db.add(file)
    chat_service.db.commit()

    files = chat_service.list_session_files(chat_session.id)
    assert len(files) > 0
    assert any(f.id == file.id for f in files)

def test_delete_chat_file(chat_service, chat_session):
    """Test deleting a chat file"""
    file = ChatFile(
        session_id=chat_session.id,
        filename="test.txt",
        content_type="text/plain",
        file_size=100,
        file_path="/path/to/test.txt",
        created_at=datetime.utcnow()
    )
    chat_service.db.add(file)
    chat_service.db.commit()

    chat_service.delete_file(file.id)
    deleted_file = chat_service.get_file(file.id)
    assert deleted_file.deleted_at is not None

def test_get_nonexistent_session(chat_service):
    """Test retrieving a non-existent chat session"""
    session = chat_service.get_session(999999)
    assert session is None

def test_get_deleted_session(chat_service, chat_session):
    """Test retrieving a deleted chat session"""
    chat_service.delete_session(chat_session.id)
    session = chat_service.get_session(chat_session.id)
    assert session.deleted_at is not None
    
def test_list_empty_user_sessions(chat_service):
    """Test listing chat sessions for a user with no sessions"""
    sessions = chat_service.list_user_sessions(user_id=999999)
    assert len(sessions) == 0

def test_get_nonexistent_file(chat_service):
    """Test retrieving a non-existent chat file"""
    file = chat_service.get_file(999999)
    assert file is None

def test_list_empty_session_files(chat_service, chat_session):
    """Test listing files for a session with no files"""
    files = chat_service.list_session_files(chat_session.id)
    assert len(files) == 0

def test_upload_file_invalid_session(chat_service):
    """Test uploading a file to a non-existent session"""
    with pytest.raises(Exception):
        chat_service.upload_file(
            session_id=999999,
            filename="test.txt",
            content_type="text/plain",
            file_size=100,
            file_path="/path/to/test.txt"
        )
