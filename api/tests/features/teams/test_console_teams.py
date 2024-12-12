import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import Session, sessionmaker
from sqlalchemy.pool import StaticPool
from fastapi import status

from app.models import Team, User, TeamMember, Base
from app.core.security import get_password_hash
from app.core.auth import create_access_token

@pytest.fixture(scope="function")
def engine():
    """Create a new engine for each test function"""
    return create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
        echo=True
    )

@pytest.fixture(scope="function")
def tables(engine):
    """Create all tables for each test function"""
    Base.metadata.create_all(engine)
    yield
    Base.metadata.drop_all(engine)

@pytest.fixture(scope="function")
def session_factory(engine):
    """Create a new session factory for each test function"""
    return sessionmaker(bind=engine, autocommit=False, autoflush=False)

@pytest.fixture(scope="function")
def test_db(engine, tables, session_factory):
    """Create a new database session for each test function"""
    connection = engine.connect()
    transaction = connection.begin()
    session = session_factory()

    yield session

    session.close()
    transaction.rollback()
    connection.close()

@pytest.fixture
def test_user(test_db: Session) -> dict:
    """Create a test user"""
    user = User(
        email="test@example.com",
        username="testuser",
        hashed_password=get_password_hash("testpass123"),
        is_active=True,
        is_superadmin=True
    )
    test_db.add(user)
    test_db.commit()
    test_db.refresh(user)
    
    # Create access token
    access_token = create_access_token(data={"sub": user.email})
    
    return {
        "id": user.id,
        "email": user.email,
        "username": user.username,
        "access_token": access_token
    }

@pytest.fixture
def test_team(test_db: Session, test_user: dict) -> dict:
    """Create a test team"""
    team = Team(
        name="Test Team",
        description="A test team",
        max_members=5,
        allow_member_invite=True,
        default_member_role="member",
        isolated_data=False,
        owner_id=test_user["id"]
    )
    test_db.add(team)
    test_db.commit()
    test_db.refresh(team)
    
    # Add owner as team member
    member = TeamMember(
        team_id=team.id,
        user_id=test_user["id"],
        role="owner"
    )
    test_db.add(member)
    test_db.commit()
    
    return {
        "id": team.id,
        "name": team.name,
        "description": team.description,
        "owner_id": team.owner_id
    }

@pytest.fixture(scope="function")
def override_get_db(test_db):
    """Override the database dependency for each test function"""
    def _override_get_db():
        try:
            yield test_db
        finally:
            test_db.close()

    from app.core.database import get_db
    from app.main import app
    app.dependency_overrides[get_db] = _override_get_db
    yield
    app.dependency_overrides.clear()

def test_create_team(client: TestClient, test_db: Session, test_user: dict, override_get_db):
    """Test creating a new team"""
    response = client.post(
        "/v1/console/teams",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={
            "name": "Test Team",
            "description": "A test team",
            "max_members": 5,
            "allow_member_invite": True,
            "default_member_role": "member",
            "isolated_data": False
        }
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "Test Team"
    assert data["description"] == "A test team"
    assert data["max_members"] == 5
    assert data["allow_member_invite"] is True
    assert data["default_member_role"] == "member"
    assert data["isolated_data"] is False
    assert data["owner_id"] == test_user["id"]
    assert len(data["members"]) == 1
    assert data["members"][0]["user_id"] == test_user["id"]
    assert data["members"][0]["role"] == "owner"
    assert data["members"][0]["email"] == test_user["email"]
    assert data["members"][0]["username"] == test_user["username"]

def test_get_team(client: TestClient, test_db: Session, test_user: dict, test_team: dict, override_get_db):
    """Test retrieving a specific team"""
    response = client.get(
        f"/v1/console/teams/{test_team['id']}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == test_team["id"]
    assert data["name"] == test_team["name"]
    assert data["description"] == test_team["description"]
    assert data["owner_id"] == test_team["owner_id"]

def test_list_teams(client: TestClient, test_db: Session, test_user: dict, test_team: dict, override_get_db):
    """Test listing all teams for a user"""
    response = client.get(
        "/v1/console/teams",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["id"] == test_team["id"]
    assert data[0]["name"] == test_team["name"]
    assert data[0]["description"] == test_team["description"]
    assert data[0]["owner_id"] == test_team["owner_id"]

def test_update_team(client: TestClient, test_db: Session, test_user: dict, test_team: dict, override_get_db):
    """Test updating a team's information"""
    response = client.put(
        f"/v1/console/teams/{test_team['id']}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={
            "name": "Updated Team",
            "description": "An updated test team",
            "max_members": 10,
            "allow_member_invite": False,
            "default_member_role": "viewer",
            "isolated_data": True
        }
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == test_team["id"]
    assert data["name"] == "Updated Team"
    assert data["description"] == "An updated test team"
    assert data["max_members"] == 10
    assert data["allow_member_invite"] is False
    assert data["default_member_role"] == "viewer"
    assert data["isolated_data"] is True
    assert data["owner_id"] == test_team["owner_id"]

def test_delete_team(client: TestClient, test_db: Session, test_user: dict, test_team: dict, override_get_db):
    """Test deleting a team"""
    response = client.delete(
        f"/v1/console/teams/{test_team['id']}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["message"] == "Team deleted successfully"
    
    # Verify team no longer exists
    response = client.get(
        f"/v1/console/teams/{test_team['id']}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    assert response.status_code == 404
