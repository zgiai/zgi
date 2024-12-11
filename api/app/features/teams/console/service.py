from datetime import datetime, timedelta
from typing import List, Optional
from sqlalchemy import or_
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError
from fastapi import HTTPException

from app.core.security import get_password_hash
from app.features.teams import schemas
from app.models import Team, TeamInvitation, TeamMember, User

class TeamConsoleService:
    def __init__(self, db: Session):
        self.db = db

    def create_team(self, team: schemas.TeamCreate, owner_id: int) -> Team:
        """Create a new team"""
        db_team = Team(
            name=team.name,
            description=team.description,
            max_members=team.max_members,
            allow_member_invite=team.allow_member_invite,
            default_member_role=team.default_member_role,
            isolated_data=team.isolated_data,
            shared_api_keys=team.shared_api_keys,
            owner_id=owner_id
        )
        self.db.add(db_team)
        self.db.commit()
        self.db.refresh(db_team)
        return db_team

    def list_teams(self, skip: int = 0, limit: int = 10, search: Optional[str] = None) -> List[Team]:
        """List all teams with pagination and optional search"""
        query = self.db.query(Team)
        if search:
            query = query.filter(
                or_(
                    Team.name.ilike(f"%{search}%"),
                    Team.description.ilike(f"%{search}%")
                )
            )
        return query.offset(skip).limit(limit).all()

    def get_team(self, team_id: int) -> Optional[Team]:
        """Get a team by ID"""
        return self.db.query(Team).filter(Team.id == team_id).first()

    def update_team(self, team_id: int, team_update: schemas.TeamUpdate) -> Team:
        """Update team information"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        update_data = team_update.dict(exclude_unset=True)
        for field, value in update_data.items():
            setattr(db_team, field, value)

        self.db.commit()
        self.db.refresh(db_team)
        return db_team

    def delete_team(self, team_id: int, force: bool = False) -> None:
        """Delete a team"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        if not force and len(db_team.members) > 0:
            raise HTTPException(
                status_code=400,
                detail="Cannot delete team with members. Use force=true to override."
            )

        self.db.delete(db_team)
        self.db.commit()

    def list_team_members(self, team_id: int, skip: int = 0, limit: int = 10) -> List[dict]:
        """List all members of a team with pagination"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        return [
            {
                "user_id": member.id,
                "username": member.username,
                "email": member.email,
                "role": next(role for _, role in db_team.members if member.id == member.id)
            }
            for member in db_team.members[skip:skip + limit]
        ]

    def add_team_member(self, team_id: int, user_id: int, role: str) -> None:
        """Add a member to team"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        db_user = self.db.query(User).filter(User.id == user_id).first()
        if not db_user:
            raise HTTPException(status_code=404, detail="User not found")

        if db_user in db_team.members:
            raise HTTPException(status_code=400, detail="User is already a team member")

        if len(db_team.members) >= db_team.max_members:
            raise HTTPException(status_code=400, detail="Team has reached maximum member limit")

        db_team.members.append(db_user)
        self.db.commit()

    def update_team_member(self, team_id: int, user_id: int, new_role: str) -> None:
        """Update team member role"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        db_user = self.db.query(User).filter(User.id == user_id).first()
        if not db_user:
            raise HTTPException(status_code=404, detail="User not found")

        if db_user not in db_team.members:
            raise HTTPException(status_code=404, detail="User is not a team member")

        # Update role in the association table
        member_assoc = next(m for m in db_team.members if m.id == user_id)
        member_assoc.role = new_role
        self.db.commit()

    def remove_team_member(self, team_id: int, user_id: int) -> None:
        """Remove a member from team"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        db_user = self.db.query(User).filter(User.id == user_id).first()
        if not db_user:
            raise HTTPException(status_code=404, detail="User not found")

        if db_user not in db_team.members:
            raise HTTPException(status_code=404, detail="User is not a team member")

        db_team.members.remove(db_user)
        self.db.commit()

    def list_team_invitations(self, team_id: int, status: Optional[str] = None) -> List[TeamInvitation]:
        """List all invitations of a team with optional status filter"""
        query = self.db.query(TeamInvitation).filter(TeamInvitation.team_id == team_id)
        if status:
            query = query.filter(TeamInvitation.status == status)
        return query.all()

    def create_team_invitation(self, team_id: int, invitation: schemas.TeamInvitationCreate) -> TeamInvitation:
        """Create a team invitation"""
        db_team = self.get_team(team_id)
        if not db_team:
            raise HTTPException(status_code=404, detail="Team not found")

        # Check if invitation already exists
        existing_invitation = self.db.query(TeamInvitation).filter(
            TeamInvitation.team_id == team_id,
            TeamInvitation.invitee_email == invitation.invitee_email,
            TeamInvitation.status == "pending"
        ).first()

        if existing_invitation:
            raise HTTPException(
                status_code=400,
                detail="An active invitation already exists for this email"
            )

        db_invitation = TeamInvitation(
            team_id=team_id,
            invitee_email=invitation.invitee_email,
            role=invitation.role,
            expires_at=datetime.utcnow() + timedelta(days=7)  # 7 days expiration
        )
        self.db.add(db_invitation)
        self.db.commit()
        self.db.refresh(db_invitation)
        return db_invitation

    def cancel_team_invitation(self, team_id: int, invitation_id: int) -> None:
        """Cancel a team invitation"""
        db_invitation = self.db.query(TeamInvitation).filter(
            TeamInvitation.id == invitation_id,
            TeamInvitation.team_id == team_id
        ).first()

        if not db_invitation:
            raise HTTPException(status_code=404, detail="Invitation not found")

        if db_invitation.status != "pending":
            raise HTTPException(status_code=400, detail="Invitation is not pending")

        db_invitation.status = "cancelled"
        self.db.commit()
