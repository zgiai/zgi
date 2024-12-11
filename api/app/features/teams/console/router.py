from typing import List
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError

from app.core.database import get_db
from app.core.auth import get_current_user, require_super_admin
from app.features.teams import schemas
from app.models import Team, TeamMember, TeamInvitation, User
from datetime import datetime, timedelta

router = APIRouter()

@router.get("/teams", response_model=List[schemas.Team])
def list_teams(
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """List all teams that the current user is a member of"""
    teams = (
        db.query(Team)
        .join(TeamMember)
        .filter(TeamMember.user_id == current_user.id)
        .all()
    )
    return [
        schemas.Team(
            id=team.id,
            name=team.name,
            description=team.description,
            max_members=team.max_members,
            allow_member_invite=team.allow_member_invite,
            default_member_role=team.default_member_role,
            isolated_data=team.isolated_data,
            shared_api_keys=team.shared_api_keys,
            owner_id=team.owner_id,
            created_at=team.created_at,
            updated_at=team.updated_at,
            members=[
                schemas.TeamMember(
                    user_id=member.user_id,
                    role=member.role,
                    email=member.user.email,
                    username=member.user.username
                )
                for member in team.team_members
            ]
        )
        for team in teams
    ]

@router.get("/teams/{team_id}", response_model=schemas.TeamDetail)
def get_team_detail(
    team_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Get detailed information about a specific team"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if not team:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Team not found"
        )
    
    # Check if user is a member of the team
    member = (
        db.query(TeamMember)
        .filter(
            TeamMember.team_id == team_id,
            TeamMember.user_id == current_user.id
        )
        .first()
    )
    if not member:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Not a member of this team"
        )
    
    return schemas.TeamDetail(
        id=team.id,
        name=team.name,
        description=team.description,
        max_members=team.max_members,
        allow_member_invite=team.allow_member_invite,
        default_member_role=team.default_member_role,
        isolated_data=team.isolated_data,
        shared_api_keys=team.shared_api_keys,
        owner_id=team.owner_id,
        created_at=team.created_at,
        updated_at=team.updated_at,
        members=[
            schemas.TeamMember(
                user_id=member.user_id,
                role=member.role,
                email=member.user.email,
                username=member.user.username
            )
            for member in team.team_members
        ],
        invitations=[
            schemas.TeamInvitation(
                id=invitation.id,
                team_id=invitation.team_id,
                inviter_id=invitation.inviter_id,
                invitee_email=invitation.invitee_email,
                role=invitation.role,
                status=invitation.status,
                created_at=invitation.created_at,
                expires_at=invitation.expires_at
            )
            for invitation in team.invitations
        ]
    )

@router.put("/teams/{team_id}", response_model=schemas.Team)
def update_team(
    team_id: int,
    team_update: schemas.TeamUpdate,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Update team information"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if not team:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Team not found"
        )
    
    # Check if user is the team owner
    if team.owner_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team owner can update team information"
        )
    
    # Update team attributes
    for field, value in team_update.dict(exclude_unset=True).items():
        setattr(team, field, value)
    
    team.updated_at = datetime.utcnow()
    db.commit()
    db.refresh(team)
    
    return schemas.Team(
        id=team.id,
        name=team.name,
        description=team.description,
        max_members=team.max_members,
        allow_member_invite=team.allow_member_invite,
        default_member_role=team.default_member_role,
        isolated_data=team.isolated_data,
        shared_api_keys=team.shared_api_keys,
        owner_id=team.owner_id,
        created_at=team.created_at,
        updated_at=team.updated_at,
        members=[
            schemas.TeamMember(
                user_id=member.user_id,
                role=member.role,
                email=member.user.email,
                username=member.user.username
            )
            for member in team.team_members
        ]
    )

@router.delete("/teams/{team_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_team(
    team_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Delete a team"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if not team:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Team not found"
        )
    
    # Check if user is the team owner
    if team.owner_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team owner can delete the team"
        )
    
    # Delete all related records
    db.query(TeamInvitation).filter(TeamInvitation.team_id == team_id).delete()
    db.query(TeamMember).filter(TeamMember.team_id == team_id).delete()
    db.delete(team)
    db.commit()

@router.post("/teams", response_model=schemas.Team)
def create_team(
    team: schemas.TeamCreate,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Create a new team"""
    from .service import TeamConsoleService
    
    service = TeamConsoleService(db)
    db_team = service.create_team(team, owner_id=current_user.id)
    
    # Add the owner as a team member with 'owner' role
    member = TeamMember(
        team_id=db_team.id,
        user_id=current_user.id,
        role='owner'
    )
    db.add(member)
    db.commit()
    db.refresh(db_team)
    
    # Format the response
    return schemas.Team(
        id=db_team.id,
        name=db_team.name,
        description=db_team.description,
        max_members=db_team.max_members,
        allow_member_invite=db_team.allow_member_invite,
        default_member_role=db_team.default_member_role,
        isolated_data=db_team.isolated_data,
        shared_api_keys=db_team.shared_api_keys,
        owner_id=db_team.owner_id,
        created_at=db_team.created_at,
        updated_at=db_team.updated_at,
        members=[
            schemas.TeamMember(
                user_id=member.user_id,
                role=member.role,
                email=member.user.email,
                username=member.user.username
            )
            for member in db_team.team_members
        ]
    )

@router.post("/teams/{team_id}/members", response_model=schemas.TeamMember)
def add_team_member(
    team_id: int,
    member: schemas.TeamMemberCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Add a new member to the team"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if not team:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Team not found"
        )
    
    # Check if user has permission to add members
    admin_member = (
        db.query(TeamMember)
        .filter(
            TeamMember.team_id == team_id,
            TeamMember.user_id == current_user.id,
            TeamMember.role.in_(['owner', 'admin'])
        )
        .first()
    )
    if not admin_member:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team owner or admin can add members"
        )
    
    # Check if user exists
    user = db.query(User).filter(User.id == member.user_id).first()
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Check if user is already a member
    existing_member = (
        db.query(TeamMember)
        .filter(
            TeamMember.team_id == team_id,
            TeamMember.user_id == member.user_id
        )
        .first()
    )
    if existing_member:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="User is already a member of this team"
        )
    
    # Check team member limit
    member_count = (
        db.query(TeamMember)
        .filter(TeamMember.team_id == team_id)
        .count()
    )
    if member_count >= team.max_members:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Team has reached maximum member limit"
        )
    
    new_member = TeamMember(
        team_id=team_id,
        user_id=member.user_id,
        role=member.role
    )
    db.add(new_member)
    db.commit()
    db.refresh(new_member)
    
    return new_member

@router.delete("/teams/{team_id}/members/{user_id}", status_code=status.HTTP_204_NO_CONTENT)
def remove_team_member(
    team_id: int,
    user_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Remove a member from the team"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if not team:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Team not found"
        )
    
    # Check if user has permission to remove members
    admin_member = (
        db.query(TeamMember)
        .filter(
            TeamMember.team_id == team_id,
            TeamMember.user_id == current_user.id,
            TeamMember.role.in_(['owner', 'admin'])
        )
        .first()
    )
    if not admin_member and current_user.id != user_id:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only team owner, admin, or the member themselves can remove members"
        )
    
    # Cannot remove team owner
    if user_id == team.owner_id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot remove team owner"
        )
    
    member = (
        db.query(TeamMember)
        .filter(
            TeamMember.team_id == team_id,
            TeamMember.user_id == user_id
        )
        .first()
    )
    if not member:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Member not found"
        )
    
    db.delete(member)
    db.commit()

@router.post("/teams/{team_id}/invitations", response_model=schemas.TeamInvitation)
def create_team_invitation(
    team_id: int,
    invitation: schemas.TeamInvitationCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Create a new team invitation"""
    team = db.query(Team).filter(Team.id == team_id).first()
    if not team:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Team not found"
        )
    
    # Check if user has permission to invite members
    member = (
        db.query(TeamMember)
        .filter(
            TeamMember.team_id == team_id,
            TeamMember.user_id == current_user.id
        )
        .first()
    )
    if not member or (not team.allow_member_invite and member.role not in ['owner', 'admin']):
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="No permission to invite members"
        )
    
    # Check if invitation already exists
    existing_invitation = (
        db.query(TeamInvitation)
        .filter(
            TeamInvitation.team_id == team_id,
            TeamInvitation.invitee_email == invitation.invitee_email,
            TeamInvitation.status == 'pending'
        )
        .first()
    )
    if existing_invitation:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invitation already exists"
        )
    
    new_invitation = TeamInvitation(
        team_id=team_id,
        inviter_id=current_user.id,
        invitee_email=invitation.invitee_email,
        role=invitation.role,
        status='pending',
        created_at=datetime.utcnow(),
        expires_at=datetime.utcnow() + timedelta(days=7)
    )
    db.add(new_invitation)
    db.commit()
    db.refresh(new_invitation)
    
    # TODO: Send invitation email
    
    return new_invitation

@router.put("/teams/{team_id}/invitations/{invitation_id}", response_model=schemas.TeamInvitation)
def update_team_invitation(
    team_id: int,
    invitation_id: int,
    invitation_update: schemas.TeamInvitationUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Update team invitation status"""
    invitation = (
        db.query(TeamInvitation)
        .filter(
            TeamInvitation.id == invitation_id,
            TeamInvitation.team_id == team_id
        )
        .first()
    )
    if not invitation:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Invitation not found"
        )
    
    # Only inviter or team admin can cancel invitation
    if invitation_update.status == 'cancelled':
        member = (
            db.query(TeamMember)
            .filter(
                TeamMember.team_id == team_id,
                TeamMember.user_id == current_user.id,
                TeamMember.role.in_(['owner', 'admin'])
            )
            .first()
        )
        if current_user.id != invitation.inviter_id and not member:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Only inviter or team admin can cancel invitation"
            )
    
    # Only invited user can accept/decline invitation
    elif invitation_update.status in ['accepted', 'declined']:
        if current_user.email != invitation.invitee_email:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Only invited user can accept/decline invitation"
            )
        
        if invitation_update.status == 'accepted':
            # Check if user is already a member
            existing_member = (
                db.query(TeamMember)
                .filter(
                    TeamMember.team_id == team_id,
                    TeamMember.user_id == current_user.id
                )
                .first()
            )
            if existing_member:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="User is already a member of this team"
                )
            
            # Add user to team
            new_member = TeamMember(
                team_id=team_id,
                user_id=current_user.id,
                role=invitation.role
            )
            db.add(new_member)
    
    invitation.status = invitation_update.status
    db.commit()
    db.refresh(invitation)
    return invitation
