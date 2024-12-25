import pytest
import os
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
from pathlib import Path

from app.features.knowledge.models.knowledge import KnowledgeBase, Visibility, Status
from app.features.knowledge.models.document import Document
from tests.utils.users import create_test_user, get_test_user_token

def create_test_file(content: str) -> Path:
    """Create a test file with given content"""
    test_dir = Path("tests/temp")
    test_dir.mkdir(exist_ok=True)
    
    file_path = test_dir / "test.txt"
    with open(file_path, "w") as f:
        f.write(content)
    
    return file_path

def test_upload_document(client: TestClient, db: Session):
    # Create test user and knowledge base
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Create test file
    test_content = "This is a test document content."
    file_path = create_test_file(test_content)
    
    # Upload document
    with open(file_path, "rb") as f:
        response = client.post(
            f"/v1/knowledge/{kb.id}/documents",
            files={"file": ("test.txt", f, "text/plain")},
            headers={"Authorization": f"Bearer {token}"}
        )
    
    assert response.status_code == 201
    data = response.json()
    assert data["file_name"] == "test.txt"
    assert data["file_type"] == "text/plain"
    assert data["status"] == Status.ACTIVE
    
    # Clean up
    os.remove(file_path)

def test_batch_upload_documents(client: TestClient, db: Session):
    # Create test user and knowledge base
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Create test files
    file_paths = []
    for i in range(3):
        content = f"This is test document {i} content."
        file_path = create_test_file(content)
        file_paths.append(file_path)
    
    # Upload documents
    files = [
        ("files", (f"test{i}.txt", open(path, "rb"), "text/plain"))
        for i, path in enumerate(file_paths)
    ]
    
    response = client.post(
        f"/v1/knowledge/{kb.id}/documents/batch",
        files=files,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 201
    data = response.json()
    assert len(data) == 3
    
    # Clean up
    for file_path in file_paths:
        os.remove(file_path)

def test_list_documents(client: TestClient, db: Session):
    # Create test user and knowledge base
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Create test documents
    for i in range(3):
        doc = Document(
            kb_id=kb.id,
            file_name=f"test{i}.txt",
            file_type="text/plain",
            status=Status.ACTIVE
        )
        db.add(doc)
    db.commit()
    
    # List documents
    response = client.get(
        f"/v1/knowledge/{kb.id}/documents",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 3
    assert len(data["items"]) == 3

def test_get_document(client: TestClient, db: Session):
    # Create test user and knowledge base
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Create test document
    doc = Document(
        kb_id=kb.id,
        file_name="test.txt",
        file_type="text/plain",
        status=Status.ACTIVE
    )
    db.add(doc)
    db.commit()
    
    # Get document
    response = client.get(
        f"/v1/knowledge/documents/{doc.id}",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == doc.id
    assert data["file_name"] == doc.file_name
    assert data["file_type"] == doc.file_type

def test_update_document(client: TestClient, db: Session):
    # Create test user and knowledge base
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Create test document
    doc = Document(
        kb_id=kb.id,
        file_name="test.txt",
        file_type="text/plain",
        status=Status.ACTIVE
    )
    db.add(doc)
    db.commit()
    
    # Update document
    update_data = {
        "metadata": {"key": "value"}
    }
    
    response = client.put(
        f"/v1/knowledge/documents/{doc.id}",
        json=update_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["metadata"] == update_data["metadata"]

def test_delete_document(client: TestClient, db: Session):
    # Create test user and knowledge base
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Create test document
    doc = Document(
        kb_id=kb.id,
        file_name="test.txt",
        file_type="text/plain",
        status=Status.ACTIVE
    )
    db.add(doc)
    db.commit()
    
    # Delete document
    response = client.delete(
        f"/v1/knowledge/documents/{doc.id}",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    
    # Verify in database
    doc = db.query(Document).filter(Document.id == doc.id).first()
    assert doc.status == Status.DELETED
