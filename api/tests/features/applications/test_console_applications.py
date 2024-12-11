import pytest
from fastapi import status
from sqlalchemy.orm import Session

from app.models import User, Team, TeamMember, Application
from app.features.applications.schemas import ApplicationType, AccessLevel
from app.core.security import get_password_hash

def create_test_user(db: Session, email: str = "test@example.com", is_superuser: bool = True) -> User:
    user = User(
        email=email,
        username=email.split("@")[0],
        hashed_password=get_password_hash("password"),
        is_active=True,
        is_superuser=is_superuser
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user

def create_test_team(db: Session, owner: User, name: str = "Test Team") -> Team:
    team = Team(
        name=name,
        owner_id=owner.id,
        description="Test team description"
    )
    db.add(team)
    db.commit()
    db.refresh(team)
    
    # Add owner as team member
    team_member = TeamMember(
        team_id=team.id,
        user_id=owner.id,
        role="owner"
    )
    db.add(team_member)
    db.commit()
    return team

def create_test_application(db: Session, creator: User, team_id: int = None) -> Application:
    application = Application(
        name="Test Application",
        description="Test application description",
        type=ApplicationType.CONVERSATIONAL.value,
        access_level=AccessLevel.PRIVATE.value,
        team_id=team_id,
        created_by=creator.id
    )
    db.add(application)
    db.commit()
    db.refresh(application)
    return application

def test_create_application(client, db: Session):
    # Create test user
    user = create_test_user(db)
    
    # Login to get access token
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "test@example.com", "password": "password"}
    )
    assert response.status_code == status.HTTP_200_OK
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}
    
    # Test creating private application
    response = client.post(
        "/v1/console/applications",
        headers=headers,
        json={
            "name": "My App",
            "description": "My first application",
            "type": "conversational",
            "access_level": "private"
        }
    )
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["name"] == "My App"
    assert data["type"] == "conversational"
    assert data["access_level"] == "private"
    assert data["created_by"] == user.id

def test_create_team_application(client, db: Session):
    # Create test user and team
    user = create_test_user(db)
    team = create_test_team(db, user)
    
    # Login
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "test@example.com", "password": "password"}
    )
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}
    
    # Test creating team application
    response = client.post(
        "/v1/console/applications",
        headers=headers,
        json={
            "name": "Team App",
            "description": "Team application",
            "type": "generative",
            "access_level": "team",
            "team_id": team.id
        }
    )
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["name"] == "Team App"
    assert data["team_id"] == team.id
    assert data["access_level"] == "team"

def test_list_applications(client, db: Session):
    # Create test user and applications
    user = create_test_user(db)
    app1 = create_test_application(db, user)
    app2 = create_test_application(db, user)
    
    # Login
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "test@example.com", "password": "password"}
    )
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}
    
    # Test listing applications
    response = client.get("/v1/console/applications", headers=headers)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert len(data) == 2
    
    # Test search
    response = client.get(
        "/v1/console/applications?search=Test",
        headers=headers
    )
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert len(data) > 0
    
    # Test filtering by type
    response = client.get(
        "/v1/console/applications?type=conversational",
        headers=headers
    )
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert all(app["type"] == "conversational" for app in data)

def test_get_application(client, db: Session):
    # Create test user and application
    user = create_test_user(db)
    app = create_test_application(db, user)
    
    # Login
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "test@example.com", "password": "password"}
    )
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}
    
    # Test getting application details
    response = client.get(f"/v1/console/applications/{app.id}", headers=headers)
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["id"] == app.id
    assert data["name"] == app.name
    
    # Test getting non-existent application
    response = client.get("/v1/console/applications/9999", headers=headers)
    assert response.status_code == status.HTTP_404_NOT_FOUND

def test_update_application(client, db: Session):
    # Create test user and application
    user = create_test_user(db)
    app = create_test_application(db, user)
    
    # Login
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "test@example.com", "password": "password"}
    )
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}
    
    # Test updating application
    response = client.put(
        f"/v1/console/applications/{app.id}",
        headers=headers,
        json={
            "name": "Updated App",
            "description": "Updated description",
            "type": "function",
            "access_level": "public"
        }
    )
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["name"] == "Updated App"
    assert data["type"] == "function"
    assert data["access_level"] == "public"

def test_delete_application(client, db: Session):
    # Create test user and application
    user = create_test_user(db)
    app = create_test_application(db, user)
    
    # Login
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "test@example.com", "password": "password"}
    )
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}
    
    # Test deleting application
    response = client.delete(f"/v1/console/applications/{app.id}", headers=headers)
    assert response.status_code == status.HTTP_204_NO_CONTENT
    
    # Verify application is deleted
    response = client.get(f"/v1/console/applications/{app.id}", headers=headers)
    assert response.status_code == status.HTTP_404_NOT_FOUND

def test_team_permissions(client, db: Session):
    # Create test users and team
    owner = create_test_user(db, "owner@example.com", is_superuser=True)
    member = create_test_user(db, "member@example.com", is_superuser=False)
    team = create_test_team(db, owner)

    # Add member to team
    team_member = TeamMember(
        team_id=team.id,
        user_id=member.id,
        role="member"
    )
    db.add(team_member)
    db.commit()

    # Create team application
    app = create_test_application(db, owner, team.id)

    # Login as member
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "member@example.com", "password": "password"}
    )
    assert response.status_code == status.HTTP_403_FORBIDDEN
    assert response.json()["detail"] == "User is not authorized for console access"

    # Login as owner
    response = client.post(
        "/v1/console/auth/login",
        json={"email": "owner@example.com", "password": "password"}
    )
    assert response.status_code == status.HTTP_200_OK
    token = response.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}

    # Test owner can view team application
    response = client.get(f"/v1/console/applications/{app.id}", headers=headers)
    assert response.status_code == status.HTTP_200_OK

    # Test owner can update team application
    response = client.put(
        f"/v1/console/applications/{app.id}",
        headers=headers,
        json={"name": "Updated by owner"}
    )
    assert response.status_code == status.HTTP_200_OK

    # Test owner can delete team application
    response = client.delete(f"/v1/console/applications/{app.id}", headers=headers)
    assert response.status_code == status.HTTP_204_NO_CONTENT
