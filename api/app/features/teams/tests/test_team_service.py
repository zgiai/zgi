import pytest
from datetime import datetime
from unittest.mock import Mock, patch
from fastapi import HTTPException
from sqlalchemy.orm import Session

from app.features.teams.client.service import TeamClientService
from app.features.teams import schemas
from app.features.users.models import User
from app.features.teams.models import Team, TeamMember, TeamRole

@pytest.fixture
def mock_db():
    return Mock(spec=Session)

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
    return Team(
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
        updated_at=datetime.utcnow(),
        team_members=[]
    )

@pytest.fixture
def team_service(mock_db):
    return TeamClientService(mock_db)

@pytest.mark.asyncio
async def test_create_team(team_service, mock_db, mock_user):
    # 准备测试数据
    team_create = schemas.TeamCreate(
        name="New Team",
        description="New Team Description",
        max_members=10,
        allow_member_invite=True,
        default_member_role="member",
        isolated_data=False,
        shared_api_keys=True
    )
    
    # 设置模拟行为
    mock_db.add.return_value = None
    mock_db.commit.return_value = None
    mock_db.refresh.return_value = None
    
    # 执行测试
    result = await team_service.create_team(team_create, mock_user.id)
    
    # 验证结果
    assert result is not None
    assert result.name == team_create.name
    assert result.description == team_create.description
    mock_db.add.assert_called()
    mock_db.commit.assert_called()

@pytest.mark.asyncio
async def test_update_team(team_service, mock_db, mock_team, mock_user):
    # 准备测试数据
    team_update = schemas.TeamUpdate(
        name="Updated Team",
        description="Updated Description"
    )
    
    # 设置模拟行为
    mock_db.query.return_value.filter.return_value.first.return_value = mock_team
    mock_db.commit.return_value = None
    mock_db.refresh.return_value = None
    
    # 执行测试
    result = await team_service.update_team(1, team_update, mock_user.id)
    
    # 验证结果
    assert result is not None
    assert result.name == team_update.name
    assert result.description == team_update.description
    mock_db.commit.assert_called()

@pytest.mark.asyncio
async def test_delete_team(team_service, mock_db, mock_team, mock_user):
    # 设置模拟行为
    mock_db.query.return_value.filter.return_value.first.return_value = mock_team
    mock_db.delete.return_value = None
    mock_db.commit.return_value = None
    
    # 执行测试
    result = await team_service.delete_team(1, mock_user.id)
    
    # 验证结果
    assert result is True
    mock_db.delete.assert_called_with(mock_team)
    mock_db.commit.assert_called()

@pytest.mark.asyncio
async def test_get_team(team_service, mock_db, mock_team, mock_user):
    # 设置模拟行为
    mock_db.query.return_value.filter.return_value.first.return_value = mock_team
    
    # 执行测试
    result = await team_service.get_team(1, mock_user.id)
    
    # 验证结果
    assert result is not None
    assert result.id == mock_team.id
    assert result.name == mock_team.name

@pytest.mark.asyncio
async def test_list_teams(team_service, mock_db, mock_team, mock_user):
    # 设置模拟行为
    mock_db.query.return_value.filter.return_value.all.return_value = [mock_team]
    
    # 执行测试
    result = await team_service.get_user_teams(mock_user.id)
    
    # 验证结果
    assert len(result) == 1
    assert result[0].id == mock_team.id
    assert result[0].name == mock_team.name

@pytest.mark.asyncio
async def test_create_team_validation(team_service, mock_db, mock_user):
    # 测试无效的团队名称
    with pytest.raises(HTTPException) as exc:
        await team_service.create_team(
            schemas.TeamCreate(
                name="",  # 空名称
                description="Test Description",
                max_members=10,
                allow_member_invite=True,
                default_member_role="member",
                isolated_data=False,
                shared_api_keys=True
            ),
            mock_user.id
        )
    assert exc.value.status_code == 400
    assert "Team name cannot be empty" in str(exc.value.detail)

@pytest.mark.asyncio
async def test_update_team_not_found(team_service, mock_db, mock_user):
    # 设置模拟行为 - 团队不存在
    mock_db.query.return_value.filter.return_value.first.return_value = None
    
    # 测试更新不存在的团队
    with pytest.raises(HTTPException) as exc:
        await team_service.update_team(
            999,
            schemas.TeamUpdate(name="Updated Team"),
            mock_user.id
        )
    assert exc.value.status_code == 404
    assert "Team not found" in str(exc.value.detail)

@pytest.mark.asyncio
async def test_delete_team_not_owner(team_service, mock_db, mock_team):
    # 设置模拟行为
    mock_db.query.return_value.filter.return_value.first.return_value = mock_team
    
    # 测试非所有者删除团队
    with pytest.raises(HTTPException) as exc:
        await team_service.delete_team(1, 999)  # 使用不同的用户ID
    assert exc.value.status_code == 403
    assert "Only team owner can delete the team" in str(exc.value.detail)

@pytest.mark.asyncio
async def test_get_team_not_member(team_service, mock_db, mock_team):
    # 设置模拟行为 - 用户不是团队成员
    mock_db.query.return_value.filter.return_value.first.return_value = None
    
    # 测试非成员访问团队
    result = await team_service.get_team(1, 999)
    assert result is None
