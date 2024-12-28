import pytest
from unittest.mock import AsyncMock, Mock, patch
from sqlalchemy.orm import Session
from fastapi import HTTPException

from app.features.knowledge.models.knowledge import KnowledgeBase, Visibility, Status
from app.features.knowledge.service.knowledge import KnowledgeBaseService
from app.features.knowledge.schemas.request.knowledge import (
    KnowledgeBaseCreate,
    KnowledgeBaseUpdate,
    SearchQuery
)

pytestmark = pytest.mark.asyncio

@pytest.fixture
def mock_db():
    db = Mock(spec=Session)
    db.query = Mock()
    db.query.return_value.filter.return_value.first.return_value = Mock(
        collection_name="test_collection",
        owner_id=1
    )
    return db

@pytest.fixture
def mock_vector_db():
    mock = AsyncMock()
    mock.create_collection = AsyncMock(return_value=True)
    mock.delete_collection = AsyncMock(return_value=True)
    mock.search = AsyncMock(return_value=[])
    return mock

@pytest.fixture
def mock_embedding():
    mock = AsyncMock()
    mock.get_dimension = AsyncMock(return_value=1536)
    mock.get_embeddings = AsyncMock(return_value=[[0.1] * 1536])
    return mock

@pytest.fixture
def kb_service(mock_db, mock_vector_db, mock_embedding):
    with patch("app.features.knowledge.service.knowledge.get_vector_db_settings") as mock_vector_settings, \
         patch("app.features.knowledge.service.knowledge.get_embedding_settings") as mock_embedding_settings, \
         patch("app.features.knowledge.service.knowledge.VectorDBFactory") as mock_vector_factory, \
         patch("app.features.knowledge.service.knowledge.EmbeddingFactory") as mock_embedding_factory:
        
        mock_vector_settings.return_value.PROVIDER = "mock"
        mock_vector_settings.return_value.provider_config = {}
        mock_vector_factory.create.return_value = mock_vector_db
        
        mock_embedding_settings.return_value.PROVIDER = "mock"
        mock_embedding_settings.return_value.provider_config = {}
        mock_embedding_factory.create.return_value = mock_embedding
        
        service = KnowledgeBaseService(mock_db)
        return service

async def test_create_knowledge_base_success(kb_service, mock_db):
    # Arrange
    kb_create = KnowledgeBaseCreate(
        name="Test KB",
        description="Test Description",
        visibility=Visibility.PRIVATE
    )
    user_id = 1
    mock_db.commit = Mock()
    mock_db.refresh = Mock()
    
    # Act
    result = await kb_service.create_knowledge_base(kb_create, user_id)
    
    # Assert
    assert result.success is True
    assert result.data.name == "Test KB"
    assert result.data.description == "Test Description"
    assert result.data.visibility == Visibility.PRIVATE
    assert result.data.owner_id == user_id
    mock_db.add.assert_called_once()
    mock_db.commit.assert_called_once()
    mock_db.refresh.assert_called_once()
    kb_service.vector_db.create_collection.assert_awaited_once()

async def test_create_knowledge_base_vector_db_failure(kb_service):
    # Arrange
    kb_create = KnowledgeBaseCreate(
        name="Test KB",
        description="Test Description"
    )
    user_id = 1
    kb_service.vector_db.create_collection.return_value = False
    
    # Act & Assert
    with pytest.raises(HTTPException) as exc:
        await kb_service.create_knowledge_base(kb_create, user_id)
    assert exc.value.status_code == 500
    assert "Failed to create vector collection" in str(exc.value.detail)

async def test_get_knowledge_base_not_found(kb_service, mock_db):
    # Arrange
    kb_id = 1
    user_id = 1
    mock_db.query.return_value.filter.return_value.first.return_value = None
    
    # Act & Assert
    with pytest.raises(HTTPException) as exc:
        await kb_service.get_knowledge_base(kb_id, user_id)
    assert exc.value.status_code == 404
    assert "Knowledge base not found" in str(exc.value.detail)

async def test_get_knowledge_base_unauthorized(kb_service, mock_db):
    # Arrange
    kb_id = 1
    user_id = 1
    mock_kb = Mock()
    mock_kb.owner_id = 2  # Different user
    mock_kb.visibility = Visibility.PRIVATE
    mock_db.query.return_value.filter.return_value.first.return_value = mock_kb
    
    # Act & Assert
    with pytest.raises(HTTPException) as exc:
        await kb_service.get_knowledge_base(kb_id, user_id)
    assert exc.value.status_code == 403
    assert "Not authorized" in str(exc.value.detail)

async def test_search_success(kb_service):
    # Arrange
    kb_id = 1
    user_id = 1
    query = SearchQuery(
        text="test query",
        top_k=5
    )
    expected_results = []
    mock_kb = Mock(collection_name="test_collection", owner_id=1)
    kb_service.db.query.return_value.filter.return_value.first.return_value = mock_kb
    kb_service.vector_db.search.return_value = expected_results
    kb_service.embedding.get_embeddings.return_value = [[0.1, 0.2, 0.3]]

    # Act
    results = await kb_service.search(kb_id, query, user_id)
    
    # Assert
    assert results == expected_results
    kb_service.embedding.get_embeddings.assert_awaited_once_with(["test query"])
    kb_service.vector_db.search.assert_awaited_once_with(
        collection_name=f"kb_{kb_id}",
        query_vector=[0.1, 0.2, 0.3],
        top_k=5,
        filters=None
    )
