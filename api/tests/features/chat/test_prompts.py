import pytest
from fastapi import FastAPI
from httpx import AsyncClient
from sqlalchemy.orm import Session

from app.models.prompt import Prompt
from app.features.chat.prompt_service import PromptService

@pytest.fixture
def prompt_service(db: Session):
    return PromptService(db)

@pytest.mark.asyncio
async def test_create_prompt(client: AsyncClient, test_user_token: str):
    """Test creating a new prompt"""
    response = await client.post(
        "/v1/chat/prompts",
        headers={"Authorization": f"Bearer {test_user_token}"},
        json={
            "title": "Test Prompt",
            "content": "This is a test prompt",
            "scenario": "testing",
            "description": "A prompt for testing"
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert data["title"] == "Test Prompt"
    assert data["content"] == "This is a test prompt"
    assert not data["is_template"]

@pytest.mark.asyncio
async def test_list_prompts(client: AsyncClient, test_user_token: str, prompt_service: PromptService, test_user):
    """Test listing prompts with pagination"""
    # Create some test prompts
    for i in range(5):
        prompt_service.create_prompt(
            test_user.id,
            PromptCreate(
                title=f"Prompt {i}",
                content=f"Content {i}",
                scenario="testing"
            )
        )

    response = await client.get(
        "/v1/chat/prompts",
        headers={"Authorization": f"Bearer {test_user_token}"},
        params={"page": 1, "page_size": 3}
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data["items"]) == 3
    assert data["total"] == 5

@pytest.mark.asyncio
async def test_update_prompt(client: AsyncClient, test_user_token: str, prompt_service: PromptService, test_user):
    """Test updating a prompt"""
    # Create a test prompt
    prompt = prompt_service.create_prompt(
        test_user.id,
        PromptCreate(
            title="Original Title",
            content="Original Content",
            scenario="testing"
        )
    )

    response = await client.put(
        f"/v1/chat/prompts/{prompt.id}",
        headers={"Authorization": f"Bearer {test_user_token}"},
        json={
            "title": "Updated Title",
            "content": "Updated Content"
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert data["title"] == "Updated Title"
    assert data["content"] == "Updated Content"

@pytest.mark.asyncio
async def test_delete_prompt(client: AsyncClient, test_user_token: str, prompt_service: PromptService, test_user):
    """Test deleting a prompt"""
    # Create a test prompt
    prompt = prompt_service.create_prompt(
        test_user.id,
        PromptCreate(
            title="Test Prompt",
            content="Test Content",
            scenario="testing"
        )
    )

    response = await client.delete(
        f"/v1/chat/prompts/{prompt.id}",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert response.status_code == 200

    # Verify prompt is deleted
    response = await client.get(
        f"/v1/chat/prompts/{prompt.id}",
        headers={"Authorization": f"Bearer {test_user_token}"}
    )
    assert response.status_code == 404

@pytest.mark.asyncio
async def test_preview_prompt(client: AsyncClient, test_user_token: str):
    """Test previewing a prompt"""
    response = await client.post(
        "/v1/chat/prompts/preview",
        headers={"Authorization": f"Bearer {test_user_token}"},
        json={
            "content": "Generate a test response",
            "variables": {"name": "Test"}
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert "preview" in data
    assert "tokens_used" in data
    assert "model_used" in data
