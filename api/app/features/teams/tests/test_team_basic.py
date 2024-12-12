import pytest
from datetime import datetime
from unittest.mock import Mock, PropertyMock, MagicMock, patch
from sqlalchemy.orm import Session
from fastapi import HTTPException

from app.features.teams import schemas
from app.features.teams.client.service import TeamClientService
from app.features.teams.models import Team, TeamMember, TeamRole

@pytest.fixture
def mock_user():
    user = Mock()
    user.id = 1
    user.email = "test@example.com"
    user.username = "test_user"
    user.is_active = True
    user.created_at = datetime.utcnow()
    return user

@pytest.fixture
def mock_team():
    team = Mock()
    team.id = 1
    team.name = "Test Team"
    team.description = "Test Team Description"
    team.max_members = 10
    team.allow_member_invite = True
    team.default_member_role = "member"
    team.isolated_data = False
    team.shared_api_keys = True
    team.owner_id = 1
    team.created_at = datetime.utcnow()
    team.updated_at = datetime.utcnow()
    
    member = Mock()
    member.team_id = 1
    member.user_id = 1
    member.role = "owner"
    member.created_at = datetime.utcnow()
    
    team.team_members = [member]
    return team

@pytest.fixture
def mock_db_session():
    session = MagicMock()
    
    # Mock query builder
    query = MagicMock()
    session.query.return_value = query
    query.filter.return_value = query
    query.first.return_value = None
    query.all.return_value = []
    
    # Mock other session methods
    session.commit = MagicMock()
    session.refresh = MagicMock()
    session.add = MagicMock()
    session.delete = MagicMock()
    
    return session

@pytest.fixture
def team_service(mock_db_session):
    return TeamClientService(mock_db_session)

@patch('app.features.teams.client.service.Team')
@patch('app.features.teams.client.service.TeamMember')
def test_create_team(mock_team_member_class, mock_team_class, team_service, mock_user, mock_db_session):
    # Prepare test data
    team_create = schemas.TeamCreate(
        name="New Team",
        description="New Team Description",
        max_members=5,
        allow_member_invite=True,
        default_member_role="member",
        isolated_data=False,
        shared_api_keys=True
    )
    
    # Mock Team instance
    mock_team = Mock()
    mock_team.id = 1
    mock_team.name = team_create.name
    mock_team.description = team_create.description
    mock_team.max_members = team_create.max_members
    mock_team.owner_id = mock_user.id
    mock_team.created_at = datetime.utcnow()
    mock_team.updated_at = datetime.utcnow()
    mock_team_class.return_value = mock_team
    
    # Mock TeamMember instance
    mock_member = Mock()
    mock_member.team_id = mock_team.id
    mock_member.user_id = mock_user.id
    mock_member.role = TeamRole.OWNER
    mock_member.created_at = datetime.utcnow()
    mock_team_member_class.return_value = mock_member
    
    # Create team
    team = team_service.create_team(team_create, mock_user.id)
    
    # Verify team was created
    assert team.name == team_create.name
    assert team.description == team_create.description
    assert team.max_members == team_create.max_members
    assert team.owner_id == mock_user.id
    
    # Verify database calls
    mock_db_session.add.assert_called()
    mock_db_session.commit.assert_called()
    mock_db_session.refresh.assert_called()

def test_update_team(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries
    mock_db_session.query.return_value.filter.return_value.all.return_value = [(mock_team.id,)]
    mock_db_session.query.return_value.filter.return_value.first.side_effect = [
        mock_team,  # For get_team
        Mock(team_id=mock_team.id, user_id=mock_user.id, role="owner")  # For get_member_role
    ]
    
    # Prepare update data
    team_update = schemas.TeamUpdate(
        name="Updated Team",
        description="Updated Description"
    )
    
    # Update team
    updated_team = team_service.update_team(mock_team.id, team_update, mock_user.id)
    
    # Verify updates
    assert updated_team.name == team_update.name
    assert updated_team.description == team_update.description
    assert updated_team.id == mock_team.id
    
    # Verify database calls
    mock_db_session.commit.assert_called_once()
    mock_db_session.refresh.assert_called_once()

def test_delete_team(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries
    mock_db_session.query.return_value.filter.return_value.all.return_value = [(mock_team.id,)]
    mock_db_session.query.return_value.filter.return_value.first.side_effect = [
        mock_team,  # For get_team
        Mock(team_id=mock_team.id, user_id=mock_user.id, role="owner")  # For get_member_role
    ]
    
    # Delete team
    assert team_service.delete_team(mock_team.id, mock_user.id) is True
    
    # Verify database calls
    mock_db_session.delete.assert_called_once_with(mock_team)
    mock_db_session.commit.assert_called_once()

def test_get_team(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries
    mock_db_session.query.return_value.filter.return_value.all.return_value = [(mock_team.id,)]
    mock_db_session.query.return_value.filter.return_value.first.return_value = mock_team
    
    # Get team
    team = team_service.get_team(mock_team.id, mock_user.id)
    
    # Verify team details
    assert team.id == mock_team.id
    assert team.name == mock_team.name
    assert team.description == mock_team.description

def test_list_teams(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries
    mock_db_session.query.return_value.filter.return_value.all.side_effect = [
        [(mock_team.id,)],  # For _get_user_team_ids
        [mock_team]  # For get_user_teams
    ]
    
    # Get user's teams
    teams = team_service.get_user_teams(mock_user.id)
    
    # Verify teams list
    assert len(teams) == 1
    assert teams[0].id == mock_team.id
    assert teams[0].name == mock_team.name

def test_get_team_not_member(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries to return no team (user is not a member)
    mock_db_session.query.return_value.filter.return_value.all.return_value = []
    mock_db_session.query.return_value.filter.return_value.first.return_value = None
    
    # Try to get team without being a member
    team = team_service.get_team(mock_team.id, mock_user.id)
    assert team is None

def test_update_team_not_owner(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries
    mock_db_session.query.return_value.filter.return_value.all.return_value = [(mock_team.id,)]
    mock_db_session.query.return_value.filter.return_value.first.side_effect = [
        mock_team,  # For get_team
        Mock(team_id=mock_team.id, user_id=mock_user.id, role="member")  # For get_member_role
    ]
    
    # Try to update team without being owner
    team_update = schemas.TeamUpdate(name="Updated Team")
    with pytest.raises(HTTPException) as exc:
        team_service.update_team(mock_team.id, team_update, mock_user.id)
    assert exc.value.status_code == 403

def test_delete_team_not_owner(team_service, mock_team, mock_user, mock_db_session):
    # Mock database queries
    mock_db_session.query.return_value.filter.return_value.all.return_value = [(mock_team.id,)]
    mock_db_session.query.return_value.filter.return_value.first.side_effect = [
        mock_team,  # For get_team
        Mock(team_id=mock_team.id, user_id=mock_user.id, role="member")  # For get_member_role
    ]
    
    # Try to delete team without being owner
    with pytest.raises(HTTPException) as exc:
        team_service.delete_team(mock_team.id, mock_user.id)
    assert exc.value.status_code == 403
