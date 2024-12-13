import pytest
from datetime import datetime
from sqlalchemy.orm import Session
from fastapi.testclient import TestClient

from app.core.config import settings
from app.features.api_keys.models import APIKey
from app.features.projects.models import Project
from app.features.organizations.models import Organization
from app.features.users.models import User

def test_create_api_key(
    client: TestClient,
    db: Session,
    normal_user_token_headers: dict,
    test_project: Project
):
    """Test creating a new API key"""
    data = {
        "name": "Test API Key"
    }
    response = client.post(
        f"/v1/projects/{test_project.uuid}/api-keys",
        headers=normal_user_token_headers,
        json=data
    )
    assert response.status_code == 200
    content = response.json()
    assert content["name"] == data["name"]
    assert content["project_uuid"] == str(test_project.uuid)
    assert content["key"].startswith("zgi_")
    assert content["is_active"] is True

    # Verify in database
    db_key = db.query(APIKey).filter(APIKey.uuid == content["uuid"]).first()
    assert db_key is not None
    assert db_key.name == data["name"]
    assert db_key.project_id == test_project.id
    assert db_key.is_active is True

def test_create_api_key_project_not_found(
    client: TestClient,
    normal_user_token_headers: dict
):
    """Test creating an API key for non-existent project"""
    data = {
        "name": "Test API Key"
    }
    response = client.post(
        "/v1/projects/non-existent-uuid/api-keys",
        headers=normal_user_token_headers,
        json=data
    )
    assert response.status_code == 404
    assert response.json()["detail"] == "Project not found"

def test_list_api_keys(
    client: TestClient,
    db: Session,
    normal_user_token_headers: dict,
    test_project: Project,
    test_user: User
):
    """Test listing API keys for a project"""
    # Create some test API keys
    api_keys = []
    for i in range(3):
        key = APIKey(
            name=f"Test Key {i}",
            key=f"zgi_test_key_{i}",
            project_id=test_project.id,
            created_by=test_user.id
        )
        db.add(key)
        api_keys.append(key)
    db.commit()

    response = client.get(
        f"/v1/projects/{test_project.uuid}/api-keys",
        headers=normal_user_token_headers
    )
    assert response.status_code == 200
    content = response.json()
    assert len(content) == 3
    assert all(key["project_uuid"] == str(test_project.uuid) for key in content)
    assert all(key["is_active"] is True for key in content)

def test_list_api_keys_empty(
    client: TestClient,
    normal_user_token_headers: dict,
    test_project: Project
):
    """Test listing API keys when none exist"""
    response = client.get(
        f"/v1/projects/{test_project.uuid}/api-keys",
        headers=normal_user_token_headers
    )
    assert response.status_code == 200
    assert response.json() == []

def test_disable_api_key(
    client: TestClient,
    db: Session,
    normal_user_token_headers: dict,
    test_project: Project,
    test_user: User
):
    """Test disabling an API key"""
    # Create a test API key
    api_key = APIKey(
        name="Test Key",
        key="zgi_test_key",
        project_id=test_project.id,
        created_by=test_user.id
    )
    db.add(api_key)
    db.commit()

    response = client.post(
        f"/v1/projects/{test_project.uuid}/api-keys/{api_key.uuid}/disable",
        headers=normal_user_token_headers
    )
    assert response.status_code == 200
    assert response.json()["message"] == "API key disabled successfully"

    # Verify in database
    db.refresh(api_key)
    assert api_key.is_active is False

def test_disable_non_existent_api_key(
    client: TestClient,
    normal_user_token_headers: dict,
    test_project: Project
):
    """Test disabling a non-existent API key"""
    response = client.post(
        f"/v1/projects/{test_project.uuid}/api-keys/non-existent-uuid/disable",
        headers=normal_user_token_headers
    )
    assert response.status_code == 404
    assert response.json()["detail"] == "API key not found"

def test_delete_api_key(
    client: TestClient,
    db: Session,
    normal_user_token_headers: dict,
    test_project: Project,
    test_user: User
):
    """Test deleting an API key"""
    # Create a test API key
    api_key = APIKey(
        name="Test Key",
        key="zgi_test_key",
        project_id=test_project.id,
        created_by=test_user.id
    )
    db.add(api_key)
    db.commit()

    response = client.delete(
        f"/v1/projects/{test_project.uuid}/api-keys/{api_key.uuid}",
        headers=normal_user_token_headers
    )
    assert response.status_code == 200
    assert response.json()["message"] == "API key deleted successfully"

    # Verify in database
    db.refresh(api_key)
    assert api_key.deleted_at is not None

def test_delete_already_deleted_api_key(
    client: TestClient,
    db: Session,
    normal_user_token_headers: dict,
    test_project: Project,
    test_user: User
):
    """Test deleting an already deleted API key"""
    # Create a deleted API key
    api_key = APIKey(
        name="Test Key",
        key="zgi_test_key",
        project_id=test_project.id,
        created_by=test_user.id,
        deleted_at=datetime.utcnow()
    )
    db.add(api_key)
    db.commit()

    response = client.delete(
        f"/v1/projects/{test_project.uuid}/api-keys/{api_key.uuid}",
        headers=normal_user_token_headers
    )
    assert response.status_code == 404
    assert response.json()["detail"] == "API key not found"

@pytest.fixture
def test_project(db: Session, test_organization: Organization) -> Project:
    """Fixture to create a test project"""
    project = Project(
        name="Test Project",
        organization_id=test_organization.id,
        created_by=1
    )
    db.add(project)
    db.commit()
    db.refresh(project)
    return project
