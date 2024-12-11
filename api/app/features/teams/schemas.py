from datetime import datetime
from typing import Optional, List, Dict
from pydantic import BaseModel, EmailStr, Field
from enum import Enum

class TeamRole(str, Enum):
    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"
    GUEST = "guest"

class InvitationStatus(str, Enum):
    PENDING = "pending"
    ACCEPTED = "accepted"
    REJECTED = "rejected"
    EXPIRED = "expired"

# Base schemas
class TeamBase(BaseModel):
    name: str
    description: Optional[str] = None
    max_members: Optional[int] = None
    allow_member_invite: bool = True
    default_member_role: TeamRole = TeamRole.MEMBER
    isolated_data: bool = False
    shared_api_keys: bool = False

class TeamCreate(TeamBase):
    pass

class TeamUpdate(TeamBase):
    name: Optional[str] = None
    allow_member_invite: Optional[bool] = None
    default_member_role: Optional[TeamRole] = None
    isolated_data: Optional[bool] = None
    shared_api_keys: Optional[bool] = None

class TeamMemberBase(BaseModel):
    user_id: int
    role: TeamRole

class TeamMemberCreate(BaseModel):
    user_id: int
    role: TeamRole = TeamRole.MEMBER

class TeamMemberUpdate(BaseModel):
    role: TeamRole

class TeamInvitationBase(BaseModel):
    invitee_email: EmailStr
    role: TeamRole = TeamRole.MEMBER
    message: Optional[str] = None

class TeamInvitationCreate(TeamInvitationBase):
    expires_at: Optional[datetime] = None

class TeamInvitationUpdate(BaseModel):
    status: InvitationStatus

# Response schemas
class TeamMember(TeamMemberBase):
    email: str
    username: str
    joined_at: datetime = Field(alias="created_at")

    class Config:
        from_attributes = True

class TeamInvitation(TeamInvitationBase):
    id: int
    team_id: int
    inviter_id: int
    status: InvitationStatus
    created_at: datetime
    expires_at: datetime

    class Config:
        from_attributes = True

class TeamMemberStats(BaseModel):
    total_members: int
    role_distribution: Dict[TeamRole, int]
    active_invitations: int
    invitation_stats: Dict[InvitationStatus, int]

class Team(TeamBase):
    id: int
    owner_id: int
    created_at: datetime
    updated_at: datetime
    members: List[TeamMember]
    member_stats: Optional[TeamMemberStats] = None

    class Config:
        from_attributes = True

class TeamDetail(Team):
    invitations: List[TeamInvitation]

    class Config:
        from_attributes = True
