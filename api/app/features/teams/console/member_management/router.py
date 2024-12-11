from typing import List, Optional
from fastapi import APIRouter, Depends, Query, status
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.auth import get_current_user, require_super_admin
from app.features.teams import schemas
from app.models import User
from .service import TeamMemberManagementService

router = APIRouter(prefix="/v1/console/teams", tags=["team-members"])

@router.get(
    "/{team_id}/members",
    response_model=List[schemas.TeamMember]
)
def list_team_members(
    team_id: int,
    role: Optional[schemas.TeamRole] = None,
    search: Optional[str] = None,
    skip: int = Query(0, ge=0),
    limit: int = Query(100, le=1000),
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """
    List all members of a team with optional filtering by role and search.
    Supports pagination.
    """
    service = TeamMemberManagementService(db)
    return service.list_team_members(team_id, role, search, skip, limit)

@router.get(
    "/{team_id}/stats",
    response_model=schemas.TeamMemberStats
)
def get_team_member_stats(
    team_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """
    Get comprehensive statistics about team members and invitations.
    """
    service = TeamMemberManagementService(db)
    return service.get_team_member_stats(team_id)

@router.put(
    "/{team_id}/members/{member_id}",
    response_model=schemas.TeamMember
)
def update_member_role(
    team_id: int,
    member_id: int,
    role_update: schemas.TeamMemberUpdate,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """
    Update a team member's role.
    Only team owners and admins can update roles.
    """
    service = TeamMemberManagementService(db)
    return service.update_member_role(team_id, member_id, role_update, current_user.id)

@router.post(
    "/{team_id}/invitations",
    response_model=schemas.TeamInvitation,
    status_code=status.HTTP_201_CREATED
)
def create_team_invitation(
    team_id: int,
    invitation: schemas.TeamInvitationCreate,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """
    Create a new invitation for a team.
    Team owners and admins can always create invitations.
    Regular members can create invitations if team settings allow it.
    """
    service = TeamMemberManagementService(db)
    return service.create_invitation(team_id, invitation, current_user.id)

@router.put(
    "/invitations/{invitation_id}",
    response_model=schemas.TeamInvitation
)
def process_invitation(
    invitation_id: int,
    status_update: schemas.TeamInvitationUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """
    Process (accept/reject) a team invitation.
    Only the invited user can process the invitation.
    """
    service = TeamMemberManagementService(db)
    return service.process_invitation(invitation_id, status_update, current_user.id)
