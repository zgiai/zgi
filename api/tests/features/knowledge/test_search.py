import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

from app.features.knowledge.models.knowledge import KnowledgeBase, Visibility, Status
from app.features.knowledge.models.document import Document
from app.features.knowledge.schemas.request.knowledge import SearchQuery
from tests.utils.users import create_test_user, get_test_user_token

def test_search_knowledge_base(client: TestClient, db: Session):
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
    
    # Create test documents with content
    docs = [
        Document(
            kb_id=kb.id,
            file_name="test1.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="This is a document about artificial intelligence."
        ),
        Document(
            kb_id=kb.id,
            file_name="test2.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="This document discusses machine learning concepts."
        )
    ]
    for doc in docs:
        db.add(doc)
    db.commit()
    
    # Perform search
    search_query = {
        "query": "artificial intelligence",
        "filter": None,
        "top_k": 5
    }
    
    response = client.post(
        f"/v1/knowledge/{kb.id}/search",
        json=search_query,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) > 0

def test_similarity_search(client: TestClient, db: Session):
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
    docs = [
        Document(
            kb_id=kb.id,
            file_name="test1.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="Deep learning is a subset of machine learning."
        ),
        Document(
            kb_id=kb.id,
            file_name="test2.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="Neural networks are used in deep learning."
        )
    ]
    for doc in docs:
        db.add(doc)
    db.commit()
    
    # Perform similarity search
    response = client.get(
        f"/v1/knowledge/{kb.id}/documents/{docs[0].id}/similar",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) > 0

def test_semantic_search(client: TestClient, db: Session):
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
    docs = [
        Document(
            kb_id=kb.id,
            file_name="test1.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="The quick brown fox jumps over the lazy dog."
        ),
        Document(
            kb_id=kb.id,
            file_name="test2.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="A swift canine leaps across a sleepy hound."
        )
    ]
    for doc in docs:
        db.add(doc)
    db.commit()
    
    # Perform semantic search
    search_query = {
        "query": "fox jumping dog",
        "filter": None,
        "top_k": 5
    }
    
    response = client.post(
        f"/v1/knowledge/{kb.id}/semantic-search",
        json=search_query,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) > 0

def test_hybrid_search(client: TestClient, db: Session):
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
    docs = [
        Document(
            kb_id=kb.id,
            file_name="test1.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="Python is a popular programming language."
        ),
        Document(
            kb_id=kb.id,
            file_name="test2.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="Python programming is widely used in data science."
        )
    ]
    for doc in docs:
        db.add(doc)
    db.commit()
    
    # Perform hybrid search
    search_query = {
        "query": "python programming",
        "filter": None,
        "top_k": 5
    }
    weights = {
        "semantic": 0.7,
        "keyword": 0.3
    }
    
    response = client.post(
        f"/v1/knowledge/{kb.id}/hybrid-search",
        json={"query": search_query, "weights": weights},
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) > 0

def test_search_with_filters(client: TestClient, db: Session):
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
    
    # Create test documents with metadata
    docs = [
        Document(
            kb_id=kb.id,
            file_name="test1.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="Information about Python programming.",
            metadata={"category": "programming", "language": "python"}
        ),
        Document(
            kb_id=kb.id,
            file_name="test2.txt",
            file_type="text/plain",
            status=Status.ACTIVE,
            content="Information about JavaScript programming.",
            metadata={"category": "programming", "language": "javascript"}
        )
    ]
    for doc in docs:
        db.add(doc)
    db.commit()
    
    # Perform search with filter
    search_query = {
        "query": "programming",
        "filter": {"metadata.language": "python"},
        "top_k": 5
    }
    
    response = client.post(
        f"/v1/knowledge/{kb.id}/search",
        json=search_query,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data["results"]) == 1
    assert data["results"][0]["metadata"]["language"] == "python"
