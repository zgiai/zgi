import pytest
from unittest.mock import Mock, patch, mock_open
from fastapi import UploadFile, HTTPException
from sqlalchemy.orm import Session

from app.models.knowledge_base import Document, KnowledgeBase, Visibility
from app.features.knowledge.service.document import DocumentService
from app.features.knowledge.service.knowledge import KnowledgeBaseService

@pytest.fixture
def mock_db():
    return Mock(spec=Session)

@pytest.fixture
def mock_kb_service(mock_db):
    service = Mock(spec=KnowledgeBaseService)
    service.db = mock_db
    return service

@pytest.fixture
def doc_service(mock_db, mock_kb_service):
    return DocumentService(mock_db, mock_kb_service)

def test_upload_document_success(doc_service, mock_kb_service):
    # Arrange
    kb_id = 1
    user_id = 1
    file_content = b"Test content"
    file = Mock(spec=UploadFile)
    file.filename = "test.txt"
    file.read = Mock(return_value=file_content)
    
    kb = KnowledgeBase(
        id=kb_id,
        name="Test KB",
        visibility=Visibility.PUBLIC,
        owner_id=user_id,
        collection_name="test_collection"
    )
    mock_kb_service.get_knowledge_base.return_value = kb
    
    with patch("builtins.open", mock_open()):
        with patch("os.makedirs"):
            with patch("hashlib.md5") as mock_md5:
                mock_md5.return_value.hexdigest.return_value = "test_hash"
                
                # Mock text extraction and processing
                doc_service._extract_text = Mock(return_value="Test content")
                doc_service._split_text = Mock(return_value=["chunk1", "chunk2"])
                mock_kb_service.embedding.get_embeddings.return_value = [
                    [0.1, 0.2], [0.3, 0.4]
                ]
                mock_kb_service.vector_db.insert_vectors.return_value = True
                
                # Act
                result = await doc_service.upload_document(
                    kb_id,
                    file,
                    user_id,
                    {"tag": "test"}
                )
                
                # Assert
                assert result.kb_id == kb_id
                assert result.file_name == "test.txt"
                assert result.file_type == "txt"
                assert result.chunk_count == 2
                assert result.metadata == {"tag": "test"}
                
                mock_kb_service.vector_db.insert_vectors.assert_called()
                doc_service.db.add.assert_called_once()
                doc_service.db.commit.assert_called_once()

def test_upload_document_unsupported_type(doc_service):
    # Arrange
    kb_id = 1
    user_id = 1
    file = Mock(spec=UploadFile)
    file.filename = "test.xyz"
    
    # Act & Assert
    with pytest.raises(HTTPException) as exc:
        await doc_service.upload_document(kb_id, file, user_id)
    assert exc.value.status_code == 400
    assert "Unsupported file type" in str(exc.value.detail)

def test_delete_document_success(doc_service, mock_kb_service):
    # Arrange
    doc_id = 1
    user_id = 1
    kb_id = 1
    
    document = Document(
        id=doc_id,
        kb_id=kb_id,
        file_name="test.txt",
        file_path="/test/path.txt"
    )
    doc_service.db.query.return_value.filter.return_value.first.return_value = document
    
    kb = KnowledgeBase(
        id=kb_id,
        name="Test KB",
        visibility=Visibility.PUBLIC,
        owner_id=user_id,
        collection_name="test_collection"
    )
    mock_kb_service.get_knowledge_base.return_value = kb
    
    with patch("os.path.exists", return_value=True):
        with patch("os.remove") as mock_remove:
            mock_kb_service.vector_db.delete_vectors.return_value = True
            
            # Act
            result = await doc_service.delete_document(doc_id, user_id)
            
            # Assert
            assert result is True
            mock_remove.assert_called_once_with("/test/path.txt")
            mock_kb_service.vector_db.delete_vectors.assert_called_once()
            doc_service.db.delete.assert_called_once_with(document)
            doc_service.db.commit.assert_called_once()

def test_delete_document_not_found(doc_service):
    # Arrange
    doc_id = 1
    user_id = 1
    doc_service.db.query.return_value.filter.return_value.first.return_value = None
    
    # Act & Assert
    with pytest.raises(HTTPException) as exc:
        await doc_service.delete_document(doc_id, user_id)
    assert exc.value.status_code == 404
    assert "Document not found" in str(exc.value.detail)

def test_delete_document_unauthorized(doc_service, mock_kb_service):
    # Arrange
    doc_id = 1
    user_id = 1
    kb_id = 1
    
    document = Document(
        id=doc_id,
        kb_id=kb_id,
        file_name="test.txt"
    )
    doc_service.db.query.return_value.filter.return_value.first.return_value = document
    
    kb = KnowledgeBase(
        id=kb_id,
        name="Test KB",
        visibility=Visibility.PRIVATE,
        owner_id=2,  # Different user
        collection_name="test_collection"
    )
    mock_kb_service.get_knowledge_base.return_value = kb
    
    # Act & Assert
    with pytest.raises(HTTPException) as exc:
        await doc_service.delete_document(doc_id, user_id)
    assert exc.value.status_code == 403
    assert "Only the owner can delete documents" in str(exc.value.detail)
