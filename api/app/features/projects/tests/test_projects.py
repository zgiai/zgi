import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
import uuid

from app.main import app
from app.features.projects.models import Project, ProjectStatus
from app.features.organizations.models import Organization
from app.features.users.models import User
from app.core.security.auth import create_access_token

@pytest.fixture
def client():
    return TestClient(app)

@pytest.fixture
def db(client):
    from app.core.database import SessionLocal
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()

@pytest.fixture
def test_user(db: Session):
    user = User(
        email=f"test_{uuid.uuid4()}@example.com",
        username=f"test_user_{uuid.uuid4()}",
        full_name="Test User",
        hashed_password="test_password",
        is_active=True
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user

@pytest.fixture
def test_user_token(test_user: User):
    return create_access_token(test_user.id)

@pytest.fixture
def test_user_headers(test_user_token: str):
    return {"Authorization": f"Bearer {test_user_token}"}

@pytest.fixture
def test_org(db: Session, test_user: User):
    """Create a test organization"""
    org = Organization(
        name="Test Organization",
        description="Test Description",
        created_by=test_user.id
    )
    db.add(org)
    db.commit()
    db.refresh(org)
    return org

@pytest.fixture
def test_project(db: Session, test_org: Organization, test_user: User):
    """Create a test project"""
    project = Project(
        name="Test Project",
        description="Test Project Description",
        organization_id=test_org.id,
        created_by=test_user.id
    )
    db.add(project)
    db.commit()
    db.refresh(project)
    return project

def test_create_project(client: TestClient, test_org: Organization, test_user_headers: dict):
    """Test creating a new project"""
    response = client.post(
        "/v1/projects",
        headers=test_user_headers,
        json={
            "name": "New Project",
            "description": "New Project Description",
            "organization_uuid": test_org.uuid
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "New Project"
    assert data["description"] == "New Project Description"
    assert "uuid" in data

def test_list_projects(client: TestClient, test_org: Organization, test_project: Project, test_user_headers: dict):
    """Test listing all projects for an organization"""
    response = client.get(
        f"/v1/projects?organization_uuid={test_org.uuid}",
        headers=test_user_headers
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["uuid"] == test_project.uuid

def test_get_project(client: TestClient, test_project: Project, test_user_headers: dict):
    """Test getting a specific project"""
    response = client.get(
        f"/v1/projects/{test_project.uuid}",
        headers=test_user_headers
    )
    assert response.status_code == 200
    data = response.json()
    assert data["uuid"] == test_project.uuid
    assert data["name"] == test_project.name

def test_update_project(client: TestClient, test_project: Project, test_user_headers: dict):
    """Test updating a project"""
    response = client.put(
        f"/v1/projects/{test_project.uuid}",
        headers=test_user_headers,
        json={
            "name": "Updated Project",
            "description": "Updated Description"
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "Updated Project"
    assert data["description"] == "Updated Description"

def test_delete_project(client: TestClient, test_project: Project, test_user_headers: dict):
    """Test deleting a project"""
    response = client.delete(
        f"/v1/projects/{test_project.uuid}",
        headers=test_user_headers
    )
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == ProjectStatus.DELETED
