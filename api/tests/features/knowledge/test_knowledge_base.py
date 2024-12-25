import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

from app.features.knowledge.models.knowledge import KnowledgeBase, Visibility, Status
from app.features.knowledge.schemas.request.knowledge import KnowledgeBaseCreate
from tests.utils.users import create_test_user, get_test_user_token

def test_create_knowledge_base(client: TestClient, db: Session):
    # Create test user and get token
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    # Test data
    kb_data = {
        "name": "Test Knowledge Base",
        "description": "Test description",
        "visibility": "private",
        "model": "text-embedding-3-small",
        "dimension": 1536
    }
    
    # Create knowledge base
    response = client.post(
        "/v1/knowledge/",
        json=kb_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == kb_data["name"]
    assert data["description"] == kb_data["description"]
    assert data["visibility"] == kb_data["visibility"]
    assert data["owner_id"] == user.id
    assert data["status"] == Status.ACTIVE
    
    # Verify in database
    kb = db.query(KnowledgeBase).filter(KnowledgeBase.id == data["id"]).first()
    assert kb is not None
    assert kb.name == kb_data["name"]

def test_list_knowledge_bases(client: TestClient, db: Session):
    # Create test user and knowledge bases
    user = create_test_user(db)
    token = get_test_user_token(client, user)
    
    # Create multiple knowledge bases
    kbs = []
    for i in range(3):
        kb = KnowledgeBase(
            name=f"Test KB {i}",
            description=f"Description {i}",
            visibility=Visibility.PRIVATE,
            owner_id=user.id,
            model="text-embedding-3-small",
            dimension=1536
        )
        db.add(kb)
        kbs.append(kb)
    db.commit()
    
    # List knowledge bases
    response = client.get(
        "/v1/knowledge/",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 3
    assert len(data["items"]) == 3
    
    # Test pagination
    response = client.get(
        "/v1/knowledge/?skip=1&limit=2",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data["items"]) == 2

def test_get_knowledge_base(client: TestClient, db: Session):
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
    
    # Get knowledge base
    response = client.get(
        f"/v1/knowledge/{kb.id}",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == kb.id
    assert data["name"] == kb.name
    assert data["description"] == kb.description

def test_update_knowledge_base(client: TestClient, db: Session):
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
    
    # Update data
    update_data = {
        "name": "Updated KB",
        "description": "Updated description",
        "visibility": "public"
    }
    
    # Update knowledge base
    response = client.put(
        f"/v1/knowledge/{kb.id}",
        json=update_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == update_data["name"]
    assert data["description"] == update_data["description"]
    assert data["visibility"] == update_data["visibility"]
    
    # Verify in database
    kb = db.query(KnowledgeBase).filter(KnowledgeBase.id == kb.id).first()
    assert kb.name == update_data["name"]

def test_delete_knowledge_base(client: TestClient, db: Session):
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
    
    # Delete knowledge base
    response = client.delete(
        f"/v1/knowledge/{kb.id}",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    
    # Verify in database
    kb = db.query(KnowledgeBase).filter(KnowledgeBase.id == kb.id).first()
    assert kb.status == Status.DELETED

def test_unauthorized_access(client: TestClient, db: Session):
    # Create test users
    user1 = create_test_user(db, email="user1@test.com")
    user2 = create_test_user(db, email="user2@test.com")
    token2 = get_test_user_token(client, user2)
    
    # Create knowledge base owned by user1
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PRIVATE,
        owner_id=user1.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Try to access with user2
    response = client.get(
        f"/v1/knowledge/{kb.id}",
        headers={"Authorization": f"Bearer {token2}"}
    )
    
    assert response.status_code == 403

def test_public_knowledge_base_access(client: TestClient, db: Session):
    # Create test users
    user1 = create_test_user(db, email="user1@test.com")
    user2 = create_test_user(db, email="user2@test.com")
    token2 = get_test_user_token(client, user2)
    
    # Create public knowledge base owned by user1
    kb = KnowledgeBase(
        name="Test KB",
        description="Test description",
        visibility=Visibility.PUBLIC,
        owner_id=user1.id,
        model="text-embedding-3-small",
        dimension=1536
    )
    db.add(kb)
    db.commit()
    
    # Try to access with user2
    response = client.get(
        f"/v1/knowledge/{kb.id}",
        headers={"Authorization": f"Bearer {token2}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == kb.id
