import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
import os

from app.main import app
from app.models.prompt import Prompt
from app.features.chat.prompt_service import PromptService
from app.features.chat.file_service import ChatFileService

@pytest.fixture
def test_pdf(tmp_path):
    """Create a test PDF file"""
    pdf_path = tmp_path / "test.pdf"
    with open(pdf_path, "wb") as f:
        f.write(b"%PDF-1.5\n")  # Minimal valid PDF content
    return pdf_path

@pytest.mark.asyncio
async def test_file_upload_flow(client, test_user_token, test_pdf):
    """Test the complete file upload flow"""
    # Create a chat session
    session_response = await client.post(
        "/api/chat/sessions",
        headers={"Authorization": f"Bearer {test_user_token}"},
        json={"model": "gpt-3.5-turbo"}
    )
    assert session_response.status_code == 200
    session_id = session_response.json()["id"]

    # Upload PDF file
    with open(test_pdf, "rb") as f:
        upload_response = await client.post(
            f"/api/chat/sessions/{session_id}/upload",
            headers={"Authorization": f"Bearer {test_user_token}"},
            files={"file": ("test.pdf", f, "application/pdf")}
        )
    assert upload_response.status_code == 200
    assert upload_response.json()["file"]["mime_type"] == "application/pdf"

    # Verify file was processed
    file_data = upload_response.json()["file"]
    assert "extracted_text" in file_data
    assert "metadata" in file_data
    assert "page_count" in file_data["metadata"]

@pytest.mark.asyncio
async def test_prompt_management_flow(client, test_user_token):
    """Test the complete prompt management flow"""
    # Create prompt
    create_response = await client.post(
        "/api/chat/prompts",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "title": "Test Prompt",
            "content": "Test content with {variable}",
            "scenario": "testing",
            "description": "Test description"
        }
    )
    assert create_response.status_code == 200
    prompt_id = create_response.json()["id"]

    # List prompts
    list_response = await client.get(
        "/api/chat/prompts",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert list_response.status_code == 200
    assert len(list_response.json()["items"]) > 0

    # Preview prompt
    preview_response = await client.post(
        "/api/chat/prompts/preview",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "prompt_id": prompt_id,
            "variables": {"variable": "test"}
        }
    )
    assert preview_response.status_code == 200
    assert "preview" in preview_response.json()

    # Update prompt
    update_response = await client.put(
        f"/api/chat/prompts/{prompt_id}",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "title": "Updated Title",
            "content": "Updated content"
        }
    )
    assert update_response.status_code == 200
    assert update_response.json()["title"] == "Updated Title"

    # Delete prompt
    delete_response = await client.delete(
        f"/api/chat/prompts/{prompt_id}",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert delete_response.status_code == 200

    # Verify deletion
    get_response = await client.get(
        f"/api/chat/prompts/{prompt_id}",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert get_response.status_code == 404

@pytest.mark.asyncio
async def test_prompt_error_handling(client, test_user_token):
    """Test error handling in prompt management"""
    # Test invalid prompt creation
    invalid_create = await client.post(
        "/api/chat/prompts",
        headers={
            "Authorization": f"Bearer {test_user_token}",
            "Content-Type": "application/json"
        },
        json={
            "title": "",  # Invalid empty title
            "content": "Test content",
            "scenario": "testing"
        }
    )
    assert invalid_create.status_code == 422

    # Test accessing non-existent prompt
    invalid_get = await client.get(
        "/api/chat/prompts/99999",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert invalid_get.status_code == 404

    # Test unauthorized access
    unauthorized = await client.post(
        "/api/chat/prompts",
        json={
            "title": "Test Prompt",
            "content": "Test content",
            "scenario": "testing"
        }
    )
    assert unauthorized.status_code == 401

@pytest.mark.asyncio
async def test_file_error_handling(client, test_user_token, tmp_path):
    """Test error handling in file upload"""
    # Create invalid file
    invalid_file = tmp_path / "test.txt"
    with open(invalid_file, "w") as f:
        f.write("Not a PDF file")

    # Test invalid file type
    with open(invalid_file, "rb") as f:
        invalid_upload = await client.post(
            "/api/chat/sessions/1/upload",
            headers={"Authorization": f"Bearer {test_user_token}"},
            files={"file": ("test.txt", f, "text/plain")}
        )
    assert invalid_upload.status_code == 400

    # Test non-existent session
    with open(invalid_file, "rb") as f:
        invalid_session = await client.post(
            "/api/chat/sessions/99999/upload",
            headers={"Authorization": f"Bearer {test_user_token}"},
            files={"file": ("test.txt", f, "text/plain")}
        )
    assert invalid_session.status_code == 404
