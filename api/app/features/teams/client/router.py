from typing import List
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.auth import get_current_user
from app.features.teams import schemas
from app.models import User, Team, TeamMember, TeamInvitation
from app.features.teams.client.service import TeamClientService

router = APIRouter(prefix="/v1/console/teams", tags=["teams"])

@router.post("", response_model=schemas.Team)
def create_team(
    team: schemas.TeamCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """创建新团队"""
    service = TeamClientService(db)
    return service.create_team(team, current_user.id)

@router.get("", response_model=List[schemas.Team])
def list_teams(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取当前用户的所有团队"""
    service = TeamClientService(db)
    return service.get_user_teams(current_user.id)

@router.get("/{team_id}", response_model=schemas.TeamDetail)
def get_team(
    team_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取团队详细信息"""
    service = TeamClientService(db)
    team = service.get_team(team_id, current_user.id)
    if not team:
        raise HTTPException(status_code=404, detail="Team not found")
    return team

@router.put("/{team_id}", response_model=schemas.Team)
def update_team(
    team_id: int,
    team_update: schemas.TeamUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """更新团队信息"""
    service = TeamClientService(db)
    return service.update_team(team_id, team_update, current_user.id)

@router.delete("/{team_id}")
def delete_team(
    team_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """删除团队"""
    service = TeamClientService(db)
    service.delete_team(team_id, current_user.id)
    return {"message": "Team deleted successfully"}

@router.post("/{team_id}/members")
def add_team_member(
    team_id: int,
    member: schemas.TeamMemberCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """添加团队成员"""
    service = TeamClientService(db)
    service.add_team_member(team_id, member.user_id, member.role, current_user.id)
    return {"message": "Team member added successfully"}

@router.delete("/{team_id}/members/{member_id}")
def remove_team_member(
    team_id: int,
    member_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """移除团队成员"""
    service = TeamClientService(db)
    service.remove_team_member(team_id, member_id, current_user.id)
    return {"message": "Team member removed successfully"}

@router.post("/{team_id}/invitations", response_model=schemas.TeamInvitation)
def create_team_invitation(
    team_id: int,
    invitation: schemas.TeamInvitationCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """创建团队邀请"""
    service = TeamClientService(db)
    return service.create_invitation(team_id, invitation, current_user.id)
