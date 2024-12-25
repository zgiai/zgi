from datetime import datetime, timedelta
from typing import Optional, Annotated
from fastapi import Depends, HTTPException, status, Body, Request
from sqlalchemy.orm import Session

from app.core.auth import get_current_user
from app.core.database import get_sync_db
from app.features.projects.models import Project, ProjectStatus
from app.features.api_keys.models import APIKey
from app.features.organizations.models import OrganizationMember, Role
from app.features.users.models import User

from pydantic import ValidationError
from app.features.organizations.schemas import RoleCreate

async def extract_body_organization_id(request: Request) -> int:
    try:
        json_data = await request.json()
        organization_id = json_data.get("organization_id")
        return organization_id
    except (ValidationError, KeyError) as e:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=str(e)
        )

def organization_params_require_admin(
        organization_id: int,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require access"""
    org_admin_member = db.query(OrganizationMember).filter(
        OrganizationMember.organization_id == organization_id,
        OrganizationMember.user_id == current_user.id,
        OrganizationMember.is_admin == True).first()
    if not org_admin_member and not current_user.is_superuser:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Insufficient permissions. Organization admin access required."
        )
    return current_user

def organization_body_require_admin(
        organization_id: int = Depends(extract_body_organization_id),
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require access"""
    return organization_params_require_admin(organization_id, current_user, db)

def role_params_require_admin(
        role_id: int,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require access"""
    role = db.query(Role).filter(Role.id == role_id).first()
    if not role:
        raise HTTPException(status_code=404, detail="Role not found")
    return organization_params_require_admin(role.organization_id, current_user, db)

def organization_require_member_admin(
        member_id: Annotated[int, Body(embed=True)],
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require access"""
    member = db.query(OrganizationMember).filter(OrganizationMember.id == member_id).first()
    if not member:
        raise HTTPException(status_code=404, detail="Organization member not found")
    return organization_params_require_admin(member.organization_id, current_user, db)
