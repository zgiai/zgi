import pytest
from unittest.mock import Mock
from datetime import datetime

from app.features.users.models import User
from app.features.teams.models import Team, TeamMember, TeamRole

@pytest.fixture
def mock_user():
    return User(
        id=1,
        email="test@example.com",
        username="test_user",
        is_active=True,
        created_at=datetime.utcnow()
    )

@pytest.fixture
def mock_team():
    team = Team(
        id=1,
        name="Test Team",
        description="Test Team Description",
        max_members=10,
        allow_member_invite=True,
        default_member_role="member",
        isolated_data=False,
        shared_api_keys=True,
        owner_id=1,
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    team.team_members = [
        TeamMember(
            team_id=1,
            user_id=1,
            role="owner",
            created_at=datetime.utcnow()
        )
    ]
    return team

@pytest.fixture
def mock_db_session():
    session = Mock()
    session.commit = Mock()
    session.refresh = Mock()
    session.add = Mock()
    session.delete = Mock()
    return session
