from datetime import datetime, timedelta
from typing import List, Optional, Dict
from sqlalchemy.orm import Session
from sqlalchemy import and_, func
from fastapi import HTTPException, status

from app.features.teams import schemas
from app.models import Team, TeamMember, TeamInvitation, User

class TeamMemberManagementService:
    def __init__(self, db: Session):
        self.db = db

    def get_team_member_stats(self, team_id: int) -> schemas.TeamMemberStats:
        """Get comprehensive statistics about team members and invitations"""
        # Get total members count
        total_members = self.db.query(TeamMember).filter(TeamMember.team_id == team_id).count()

        # Get role distribution
        role_distribution = dict(
            self.db.query(
                TeamMember.role,
                func.count(TeamMember.id)
            ).filter(TeamMember.team_id == team_id)
            .group_by(TeamMember.role)
            .all()
        )

        # Convert role strings to enum
        role_dist = {
            schemas.TeamRole(role): count 
            for role, count in role_distribution.items()
        }

        # Fill in missing roles with 0
        for role in schemas.TeamRole:
            if role not in role_dist:
                role_dist[role] = 0

        # Get invitation statistics
        invitation_stats = dict(
            self.db.query(
                TeamInvitation.status,
                func.count(TeamInvitation.id)
            ).filter(TeamInvitation.team_id == team_id)
            .group_by(TeamInvitation.status)
            .all()
        )

        # Convert status strings to enum
        inv_stats = {
            schemas.InvitationStatus(status): count 
            for status, count in invitation_stats.items()
        }

        # Fill in missing statuses with 0
        for status in schemas.InvitationStatus:
            if status not in inv_stats:
                inv_stats[status] = 0

        # Count active invitations
        active_invitations = (
            self.db.query(TeamInvitation)
            .filter(
                and_(
                    TeamInvitation.team_id == team_id,
                    TeamInvitation.status == schemas.InvitationStatus.PENDING,
                    TeamInvitation.expires_at > datetime.utcnow()
                )
            ).count()
        )

        return schemas.TeamMemberStats(
            total_members=total_members,
            role_distribution=role_dist,
            active_invitations=active_invitations,
            invitation_stats=inv_stats
        )

    def list_team_members(
        self,
        team_id: int,
        role: Optional[schemas.TeamRole] = None,
        search: Optional[str] = None,
        skip: int = 0,
        limit: int = 100
    ) -> List[schemas.TeamMember]:
        """List team members with optional filtering and pagination"""
        query = (
            self.db.query(TeamMember)
            .join(User)
            .filter(TeamMember.team_id == team_id)
        )

        if role:
            query = query.filter(TeamMember.role == role)

        if search:
            query = query.filter(
                User.username.ilike(f"%{search}%") |
                User.email.ilike(f"%{search}%")
            )

        members = query.offset(skip).limit(limit).all()
        return [
            schemas.TeamMember(
                user_id=member.user_id,
                role=schemas.TeamRole(member.role),
                email=member.user.email,
                username=member.user.username,
                joined_at=member.created_at
            )
            for member in members
        ]

    def update_member_role(
        self,
        team_id: int,
        member_id: int,
        role_update: schemas.TeamMemberUpdate,
        current_user_id: int
    ) -> schemas.TeamMember:
        """Update a team member's role"""
        # Check if the current user has permission (must be owner or admin)
        current_member = (
            self.db.query(TeamMember)
            .filter(
                and_(
                    TeamMember.team_id == team_id,
                    TeamMember.user_id == current_user_id,
                    TeamMember.role.in_([schemas.TeamRole.OWNER, schemas.TeamRole.ADMIN])
                )
            ).first()
        )

        if not current_member:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Only team owners and admins can update member roles"
            )

        # Get the member to update
        member = (
            self.db.query(TeamMember)
            .filter(
                and_(
                    TeamMember.team_id == team_id,
                    TeamMember.user_id == member_id
                )
            ).first()
        )

        if not member:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Team member not found"
            )

        # Check role update permissions
        if member.role == schemas.TeamRole.OWNER:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Cannot change the role of the team owner"
            )

        if (current_member.role == schemas.TeamRole.ADMIN and 
            role_update.role in [schemas.TeamRole.OWNER, schemas.TeamRole.ADMIN]):
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Admins cannot promote members to owner or admin"
            )

        # Update the role
        member.role = role_update.role
        self.db.commit()
        self.db.refresh(member)

        return schemas.TeamMember(
            user_id=member.user_id,
            role=schemas.TeamRole(member.role),
            email=member.user.email,
            username=member.user.username,
            joined_at=member.created_at
        )

    def create_invitation(
        self,
        team_id: int,
        invitation: schemas.TeamInvitationCreate,
        current_user_id: int
    ) -> schemas.TeamInvitation:
        """Create a new team invitation"""
        # Check if the team exists and get its settings
        team = self.db.query(Team).filter(Team.id == team_id).first()
        if not team:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Team not found"
            )

        # Check if the current user has permission to invite
        member = (
            self.db.query(TeamMember)
            .filter(
                and_(
                    TeamMember.team_id == team_id,
                    TeamMember.user_id == current_user_id
                )
            ).first()
        )

        if not member or (
            not team.allow_member_invite and 
            member.role not in [schemas.TeamRole.OWNER, schemas.TeamRole.ADMIN]
        ):
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="No permission to create invitations"
            )

        # Check if there's an active invitation for this email
        existing_invitation = (
            self.db.query(TeamInvitation)
            .filter(
                and_(
                    TeamInvitation.team_id == team_id,
                    TeamInvitation.invitee_email == invitation.invitee_email,
                    TeamInvitation.status == schemas.InvitationStatus.PENDING,
                    TeamInvitation.expires_at > datetime.utcnow()
                )
            ).first()
        )

        if existing_invitation:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="An active invitation already exists for this email"
            )

        # Create the invitation
        expires_at = invitation.expires_at or datetime.utcnow() + timedelta(days=7)
        db_invitation = TeamInvitation(
            team_id=team_id,
            inviter_id=current_user_id,
            invitee_email=invitation.invitee_email,
            role=invitation.role,
            message=invitation.message,
            status=schemas.InvitationStatus.PENDING,
            expires_at=expires_at
        )

        self.db.add(db_invitation)
        self.db.commit()
        self.db.refresh(db_invitation)

        return schemas.TeamInvitation(
            id=db_invitation.id,
            team_id=db_invitation.team_id,
            inviter_id=db_invitation.inviter_id,
            invitee_email=db_invitation.invitee_email,
            role=schemas.TeamRole(db_invitation.role),
            message=db_invitation.message,
            status=schemas.InvitationStatus(db_invitation.status),
            created_at=db_invitation.created_at,
            expires_at=db_invitation.expires_at
        )

    def process_invitation(
        self,
        invitation_id: int,
        status_update: schemas.TeamInvitationUpdate,
        current_user_id: int
    ) -> schemas.TeamInvitation:
        """Process (accept/reject) a team invitation"""
        invitation = (
            self.db.query(TeamInvitation)
            .filter(TeamInvitation.id == invitation_id)
            .first()
        )

        if not invitation:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Invitation not found"
            )

        # Check if the invitation is still valid
        if invitation.status != schemas.InvitationStatus.PENDING:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Invitation has already been {invitation.status}"
            )

        if invitation.expires_at < datetime.utcnow():
            invitation.status = schemas.InvitationStatus.EXPIRED
            self.db.commit()
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invitation has expired"
            )

        # Get the user by email
        user = (
            self.db.query(User)
            .filter(User.id == current_user_id)
            .first()
        )

        if not user or user.email != invitation.invitee_email:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="This invitation was not sent to you"
            )

        # Update invitation status
        invitation.status = status_update.status

        # If accepted, add user to team
        if status_update.status == schemas.InvitationStatus.ACCEPTED:
            # Check if user is already a member
            existing_member = (
                self.db.query(TeamMember)
                .filter(
                    and_(
                        TeamMember.team_id == invitation.team_id,
                        TeamMember.user_id == current_user_id
                    )
                ).first()
            )

            if existing_member:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="You are already a member of this team"
                )

            # Add user as team member
            member = TeamMember(
                team_id=invitation.team_id,
                user_id=current_user_id,
                role=invitation.role
            )
            self.db.add(member)

        self.db.commit()
        self.db.refresh(invitation)

        return schemas.TeamInvitation(
            id=invitation.id,
            team_id=invitation.team_id,
            inviter_id=invitation.inviter_id,
            invitee_email=invitation.invitee_email,
            role=schemas.TeamRole(invitation.role),
            message=invitation.message,
            status=schemas.InvitationStatus(invitation.status),
            created_at=invitation.created_at,
            expires_at=invitation.expires_at
        )
