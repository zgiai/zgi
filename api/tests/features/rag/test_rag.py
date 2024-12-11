import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
import os

from app.main import app
from app.models.rag import Document, QueryLog
from app.features.rag.service import RAGService

@pytest.fixture
def test_pdf(tmp_path):
    """Create a test PDF file"""
    pdf_path = tmp_path / "test.pdf"
    with open(pdf_path, "wb") as f:
        f.write(b"%PDF-1.5\nTest content for RAG testing")  # Minimal valid PDF content
    return pdf_path

@pytest.fixture
def rag_service(db: Session):
    return RAGService(db)

@pytest.mark.asyncio
async def test_document_upload_flow(client, test_user_token, test_pdf):
    """Test the complete document upload and processing flow"""
    # Upload document
    with open(test_pdf, "rb") as f:
        response = await client.post(
            "/v1/rag/upload",
            headers={"Authorization": f"Bearer {test_user_token}"},
            files={"file": ("test.pdf", f, "application/pdf")}
        )
    assert response.status_code == 200
    doc_data = response.json()
    assert doc_data["status"] == "processing"

    # Get document details
    response = await client.get(
        f"/v1/rag/documents/{doc_data['id']}",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert response.status_code == 200
    assert response.json()["filename"] == "test.pdf"

@pytest.mark.asyncio
async def test_search_and_generate(client, test_user_token, rag_service, test_user):
    """Test the search and generate functionality"""
    # Create test document with vectors
    doc = Document(
        user_id=test_user.id,
        filename="test.pdf",
        file_path="/tmp/test.pdf",
        file_type="application/pdf",
        file_size=1000,
        content_hash="test_hash",
        status="completed",
        vector_ids=["test_vector_1", "test_vector_2"]
    )
    rag_service.db.add(doc)
    rag_service.db.commit()

    # Test search
    response = await client.post(
        "/v1/rag/search",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "query": "test query",
            "document_ids": [doc.id],
            "top_k": 3
        }
    )
    assert response.status_code == 200
    search_results = response.json()
    assert "results" in search_results

    # Test generate
    response = await client.post(
        "/v1/rag/generate",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "query": "test query",
            "context_chunks": search_results["results"],
            "document_ids": [doc.id]
        }
    )
    assert response.status_code == 200
    assert "response" in response.json()

@pytest.mark.asyncio
async def test_combined_query(client, test_user_token, rag_service, test_user):
    """Test the combined query endpoint"""
    # Create test document
    doc = Document(
        user_id=test_user.id,
        filename="test.pdf",
        file_path="/tmp/test.pdf",
        file_type="application/pdf",
        file_size=1000,
        content_hash="test_hash",
        status="completed",
        vector_ids=["test_vector_1", "test_vector_2"]
    )
    rag_service.db.add(doc)
    rag_service.db.commit()

    # Test query endpoint
    response = await client.post(
        "/v1/rag/query",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "query": "test query",
            "document_ids": [doc.id],
            "top_k": 3
        }
    )
    assert response.status_code == 200
    result = response.json()
    assert "response" in result
    assert "context_chunks" in result

@pytest.mark.asyncio
async def test_error_handling(client, test_user_token):
    """Test error handling in RAG endpoints"""
    # Test invalid file type
    with open("test.txt", "w") as f:
        f.write("Not a PDF")
    
    with open("test.txt", "rb") as f:
        response = await client.post(
            "/v1/rag/upload",
            headers={"Authorization": f"Bearer {test_user_token}"},
            files={"file": ("test.txt", f, "text/plain")}
        )
    assert response.status_code == 400

    # Test non-existent document
    response = await client.get(
        "/v1/rag/documents/99999",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert response.status_code == 404

    # Test invalid search request
    response = await client.post(
        "/v1/rag/search",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "query": "",  # Empty query
            "top_k": 3
        }
    )
    assert response.status_code == 422

    os.remove("test.txt")
