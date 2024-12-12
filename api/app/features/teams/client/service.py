from datetime import datetime, timedelta
from typing import List, Optional
from sqlalchemy.orm import Session
from sqlalchemy import and_
from sqlalchemy.exc import IntegrityError
from fastapi import HTTPException

from app.core.security import get_password_hash
from app.features.teams import schemas
from app.features.users.models import User
from app.features.teams.models import Team, TeamInvitation, TeamMember, TeamRole

class TeamClientService:
    def __init__(self, db: Session):
        self.db = db

    def create_team(self, team: schemas.TeamCreate, user_id: int) -> Team:
        db_team = Team(**team.dict(), owner_id=user_id)
        self.db.add(db_team)
        self.db.commit()
        self.db.refresh(db_team)
        
        # Add owner as a team member with 'owner' role
        self._add_team_member(db_team.id, user_id, 'owner')
        self.db.refresh(db_team)  # Refresh again to get the updated members
        return db_team

    def get_team(self, team_id: int, user_id: int) -> Optional[Team]:
        return self.db.query(Team).filter(
            and_(
                Team.id == team_id,
                Team.id.in_(self._get_user_team_ids(user_id))
            )
        ).first()

    def get_user_teams(self, user_id: int) -> List[Team]:
        return self.db.query(Team).filter(
            Team.id.in_(self._get_user_team_ids(user_id))
        ).all()

    def update_team(self, team_id: int, team_update: schemas.TeamUpdate, user_id: int) -> Team:
        db_team = self._get_team_with_permission_check(team_id, user_id, ['owner', 'admin'])
        
        for field, value in team_update.dict(exclude_unset=True).items():
            setattr(db_team, field, value)
        
        self.db.commit()
        self.db.refresh(db_team)
        return db_team

    def delete_team(self, team_id: int, user_id: int) -> bool:
        db_team = self._get_team_with_permission_check(team_id, user_id, ['owner'])
        self.db.delete(db_team)
        self.db.commit()
        return True

    def add_team_member(self, team_id: int, member_id: int, role: str, user_id: int) -> bool:
        db_team = self._get_team_with_permission_check(team_id, user_id, ['owner', 'admin'])
        
        # Check if user is already a member
        if self._is_team_member(team_id, member_id):
            raise HTTPException(status_code=400, detail="User is already a team member")
        
        # Check team member limit
        current_members = self._get_team_member_count(team_id)
        if current_members >= db_team.max_members:
            raise HTTPException(status_code=400, detail="Team has reached maximum member limit")
        
        self._add_team_member(team_id, member_id, role)
        self.db.refresh(db_team)  # Refresh team object to get updated members
        return True

    def remove_team_member(self, team_id: int, member_id: int, user_id: int) -> bool:
        db_team = self._get_team_with_permission_check(team_id, user_id, ['owner', 'admin'])
        
        # Cannot remove the owner
        if member_id == db_team.owner_id:
            raise HTTPException(status_code=400, detail="Cannot remove team owner")
        
        # If user is admin, they cannot remove other admins
        if self._get_member_role(team_id, user_id) == 'admin' and \
           self._get_member_role(team_id, member_id) == 'admin':
            raise HTTPException(status_code=403, detail="Admins cannot remove other admins")
        
        self._remove_team_member(team_id, member_id)
        return True

    def create_invitation(self, team_id: int, invitation: schemas.TeamInvitationCreate, user_id: int) -> TeamInvitation:
        db_team = self._get_team_with_permission_check(team_id, user_id, ['owner', 'admin'])
        
        # Check if invitation already exists
        existing_invitation = self.db.query(TeamInvitation).filter(
            and_(
                TeamInvitation.team_id == team_id,
                TeamInvitation.invitee_email == invitation.invitee_email,
                TeamInvitation.status == 'pending'
            )
        ).first()
        
        if existing_invitation:
            raise HTTPException(status_code=400, detail="Invitation already exists")
        
        # Create new invitation
        db_invitation = TeamInvitation(
            team_id=team_id,
            inviter_id=user_id,
            invitee_email=invitation.invitee_email,
            role=invitation.role,
            status='pending',
            expires_at=datetime.utcnow() + timedelta(days=7)
        )
        self.db.add(db_invitation)
        self.db.commit()
        self.db.refresh(db_invitation)
        return db_invitation

    def _get_team_with_permission_check(self, team_id: int, user_id: int, allowed_roles: List[str]) -> Team:
        db_team = self.get_team(team_id, user_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")
        
        user_role = self._get_member_role(team_id, user_id)
        if user_role not in allowed_roles:
            raise HTTPException(status_code=403, detail="Insufficient permissions")
        
        return db_team

    def _get_user_team_ids(self, user_id: int) -> List[int]:
        return [
            team_id for (team_id,) in
            self.db.query(TeamMember.team_id).filter(TeamMember.user_id == user_id).all()
        ]

    def _is_team_member(self, team_id: int, user_id: int) -> bool:
        return self.db.query(TeamMember).filter(
            and_(
                TeamMember.team_id == team_id,
                TeamMember.user_id == user_id
            )
        ).first() is not None

    def _get_member_role(self, team_id: int, user_id: int) -> Optional[str]:
        member = self.db.query(TeamMember).filter(
            and_(
                TeamMember.team_id == team_id,
                TeamMember.user_id == user_id
            )
        ).first()
        return member.role if member else None

    def _get_team_member_count(self, team_id: int) -> int:
        return self.db.query(TeamMember).filter(TeamMember.team_id == team_id).count()

    def _add_team_member(self, team_id: int, user_id: int, role: str) -> None:
        # Use ORM instead of raw SQL to properly handle relationships
        member = TeamMember(
            team_id=team_id,
            user_id=user_id,
            role=role
        )
        self.db.add(member)
        self.db.commit()

    def _remove_team_member(self, team_id: int, user_id: int) -> None:
        member = self.db.query(TeamMember).filter(
            and_(
                TeamMember.team_id == team_id,
                TeamMember.user_id == user_id
            )
        ).first()
        if member:
            self.db.delete(member)
            self.db.commit()
