import pytest
from datetime import datetime, timedelta
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

from app.models import User, Team, TeamMember, TeamInvitation
from app.features.teams import schemas

@pytest.fixture
def test_team(test_db: Session, test_user: dict) -> dict:
    """Create a test team with the test user as owner"""
    team = Team(
        name="Test Team",
        description="Test Team Description",
        max_members=5,
        allow_member_invite=True,
        default_member_role="member",
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
        "owner_id": team.owner_id
    }

@pytest.fixture
def test_member(test_db: Session) -> dict:
    """Create a test member user"""
    user = User(
        email="member@example.com",
        username="testmember",
        hashed_password="$2b$12$yDM52MXdWvtaLlO8fekfyup4/pn7P7ZZ18lBFsYnFJbLwsMHECNwW",
        is_active=True,
        is_superadmin=False
    )
    test_db.add(user)
    test_db.commit()
    test_db.refresh(user)
    return {
        "id": user.id,
        "email": user.email,
        "username": user.username
    }

@pytest.fixture
def test_admin(test_db: Session, test_team: dict) -> dict:
    """Create a test admin user and add them to the team"""
    user = User(
        email="admin@example.com",
        username="testadmin",
        hashed_password="$2b$12$yDM52MXdWvtaLlO8fekfyup4/pn7P7ZZ18lBFsYnFJbLwsMHECNwW",
        is_active=True,
        is_superadmin=False
    )
    test_db.add(user)
    test_db.commit()
    test_db.refresh(user)

    # Add as team admin
    member = TeamMember(
        team_id=test_team["id"],
        user_id=user.id,
        role="admin"
    )
    test_db.add(member)
    test_db.commit()

    return {
        "id": user.id,
        "email": user.email,
        "username": user.username
    }

def test_list_team_members(
    client: TestClient,
    test_db: Session,
    test_user: dict,
    test_team: dict,
    test_member: dict,
    test_admin: dict
):
    """Test listing team members with various filters"""
    # Add test member to team
    member = TeamMember(
        team_id=test_team["id"],
        user_id=test_member["id"],
        role="member"
    )
    test_db.add(member)
    test_db.commit()

    # Test listing all members
    response = client.get(
        f"/v1/console/teams/{test_team['id']}/members",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 3  # owner, admin, and member

    # Test filtering by role
    response = client.get(
        f"/v1/console/teams/{test_team['id']}/members?role=member",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["role"] == "member"

    # Test search by username
    response = client.get(
        f"/v1/console/teams/{test_team['id']}/members?search=testmember",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["username"] == "testmember"

def test_get_team_member_stats(
    client: TestClient,
    test_db: Session,
    test_user: dict,
    test_team: dict,
    test_member: dict,
    test_admin: dict
):
    """Test getting team member statistics"""
    # Add test member to team
    member = TeamMember(
        team_id=test_team["id"],
        user_id=test_member["id"],
        role="member"
    )
    test_db.add(member)

    # Create some invitations
    invitation1 = TeamInvitation(
        team_id=test_team["id"],
        inviter_id=test_user["id"],
        invitee_email="pending@example.com",
        role="member",
        status="pending",
        expires_at=datetime.utcnow() + timedelta(days=7)
    )
    invitation2 = TeamInvitation(
        team_id=test_team["id"],
        inviter_id=test_user["id"],
        invitee_email="accepted@example.com",
        role="member",
        status="accepted",
        expires_at=datetime.utcnow() + timedelta(days=7)
    )
    test_db.add_all([invitation1, invitation2])
    test_db.commit()

    response = client.get(
        f"/v1/console/teams/{test_team['id']}/stats",
        headers={"Authorization": f"Bearer {test_user['access_token']}"}
    )
    assert response.status_code == 200
    data = response.json()

    assert data["total_members"] == 3
    assert data["role_distribution"]["owner"] == 1
    assert data["role_distribution"]["admin"] == 1
    assert data["role_distribution"]["member"] == 1
    assert data["active_invitations"] == 1
    assert data["invitation_stats"]["pending"] == 1
    assert data["invitation_stats"]["accepted"] == 1

def test_update_member_role(
    client: TestClient,
    test_db: Session,
    test_user: dict,
    test_team: dict,
    test_member: dict
):
    """Test updating a team member's role"""
    # Add test member to team
    member = TeamMember(
        team_id=test_team["id"],
        user_id=test_member["id"],
        role="member"
    )
    test_db.add(member)
    test_db.commit()

    # Test updating role to admin
    response = client.put(
        f"/v1/console/teams/{test_team['id']}/members/{test_member['id']}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={"role": "admin"}
    )
    assert response.status_code == 200
    data = response.json()
    assert data["role"] == "admin"

    # Test that non-owners cannot promote to owner
    response = client.put(
        f"/v1/console/teams/{test_team['id']}/members/{test_member['id']}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={"role": "owner"}
    )
    assert response.status_code == 403

def test_create_and_process_invitation(
    client: TestClient,
    test_db: Session,
    test_user: dict,
    test_team: dict,
    test_member: dict
):
    """Test creating and processing team invitations"""
    # Create invitation
    response = client.post(
        f"/v1/console/teams/{test_team['id']}/invitations",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={
            "invitee_email": "newinvite@example.com",
            "role": "member",
            "message": "Please join our team!"
        }
    )
    assert response.status_code == 201
    data = response.json()
    invitation_id = data["id"]
    assert data["status"] == "pending"
    assert data["message"] == "Please join our team!"

    # Create a new user for the invitee
    invitee = User(
        email="newinvite@example.com",
        username="newinvitee",
        hashed_password="$2b$12$yDM52MXdWvtaLlO8fekfyup4/pn7P7ZZ18lBFsYnFJbLwsMHECNwW",
        is_active=True,
        is_superadmin=False
    )
    test_db.add(invitee)
    test_db.commit()
    test_db.refresh(invitee)

    # Process invitation (accept)
    response = client.put(
        f"/v1/console/teams/invitations/{invitation_id}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={"status": "accepted"}
    )
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "accepted"

    # Verify member was added to team
    member = test_db.query(TeamMember).filter(
        TeamMember.team_id == test_team["id"],
        TeamMember.user_id == invitee.id
    ).first()
    assert member is not None
    assert member.role == "member"

def test_invitation_expiration(
    client: TestClient,
    test_db: Session,
    test_user: dict,
    test_team: dict
):
    """Test handling of expired invitations"""
    # Create an expired invitation
    invitation = TeamInvitation(
        team_id=test_team["id"],
        inviter_id=test_user["id"],
        invitee_email="expired@example.com",
        role="member",
        status="pending",
        expires_at=datetime.utcnow() - timedelta(days=1)
    )
    test_db.add(invitation)
    test_db.commit()
    test_db.refresh(invitation)

    # Try to accept expired invitation
    response = client.put(
        f"/v1/console/teams/invitations/{invitation.id}",
        headers={"Authorization": f"Bearer {test_user['access_token']}"},
        json={"status": "accepted"}
    )
    assert response.status_code == 400
    assert "expired" in response.json()["detail"].lower()

def test_member_permissions(
    client: TestClient,
    test_db: Session,
    test_user: dict,
    test_team: dict,
    test_member: dict,
    test_admin: dict
):
    """Test permission restrictions for different member roles"""
    # Add test member to team
    member = TeamMember(
        team_id=test_team["id"],
        user_id=test_member["id"],
        role="member"
    )
    test_db.add(member)
    test_db.commit()

    # Get access token for test member
    response = client.post(
        "/v1/auth/login",
        json={
            "email": test_member["email"],
            "password": "testpassword123"
        }
    )
    member_token = response.json()["access_token"]

    # Test that regular member cannot update roles
    response = client.put(
        f"/v1/console/teams/{test_team['id']}/members/{test_admin['id']}",
        headers={"Authorization": f"Bearer {member_token}"},
        json={"role": "member"}
    )
    assert response.status_code == 403

    # Test that regular member cannot create invitations when not allowed
    team = test_db.query(Team).filter(Team.id == test_team["id"]).first()
    team.allow_member_invite = False
    test_db.commit()

    response = client.post(
        f"/v1/console/teams/{test_team['id']}/invitations",
        headers={"Authorization": f"Bearer {member_token}"},
        json={
            "invitee_email": "test@example.com",
            "role": "member"
        }
    )
    assert response.status_code == 403
