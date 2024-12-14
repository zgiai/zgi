import pytest
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select
from app.core.database import Base, get_db, handle_db_operation
from app.features.chat.models.chat import ChatSession
from fastapi import HTTPException

@pytest.mark.asyncio
async def test_get_db(async_session):
    """Test database session management"""
    db_gen = get_db()
    session = await anext(db_gen)
    
    assert isinstance(session, AsyncSession)
    try:
        await anext(db_gen)
    except StopAsyncIteration:
        pass

@pytest.mark.asyncio
async def test_handle_db_operation_success(async_session):
    """Test successful database operation"""
    async def sample_operation():
        return "success"
    
    result = await handle_db_operation(sample_operation())
    assert result == "success"

@pytest.mark.asyncio
async def test_handle_db_operation_failure(async_session):
    """Test database operation failure"""
    async def failing_operation():
        raise Exception("Database error")
    
    with pytest.raises(HTTPException) as exc_info:
        await handle_db_operation(failing_operation())
    assert exc_info.value.status_code == 500

@pytest.mark.asyncio
async def test_model_to_dict(async_session):
    """Test model to dictionary conversion"""
    # Create a sample chat session
    session = ChatSession(
        user_id=1,
        model_id=1
    )
    
    # Convert to dictionary
    session_dict = session.to_dict()
    
    # Check essential fields
    assert isinstance(session_dict, dict)
    assert session_dict['user_id'] == 1
    assert session_dict['model_id'] == 1

@pytest.mark.asyncio
async def test_model_repr(async_session):
    """Test model string representation"""
    # Create a sample chat session
    session = ChatSession(
        user_id=1,
        model_id=1
    )
    
    # Get string representation
    session_str = str(session)
    
    # Check if string contains essential information
    assert 'ChatSession' in session_str
    assert 'user_id=1' in session_str
    assert 'model_id=1' in session_str

@pytest.mark.asyncio
async def test_database_session_commit(async_session):
    """Test database session commit"""
    # Create a sample chat session
    session = ChatSession(
        user_id=1,
        model_id=1
    )
    
    # Add and commit
    async_session.add(session)
    await async_session.commit()
    await async_session.refresh(session)
    
    # Verify session was saved
    result = await async_session.execute(
        select(ChatSession).filter(ChatSession.id == session.id)
    )
    saved_session = result.scalar_one()
    assert saved_session.user_id == 1
    assert saved_session.model_id == 1

@pytest.mark.asyncio
async def test_database_session_rollback(async_session):
    """Test database session rollback"""
    # Create a sample chat session
    session = ChatSession(
        user_id=1,
        model_id=1
    )
    
    # Add but don't commit
    async_session.add(session)
    
    # Rollback
    await async_session.rollback()
    
    # Verify session was not saved
    result = await async_session.execute(
        select(ChatSession).filter(ChatSession.user_id == 1)
    )
    assert result.first() is None
