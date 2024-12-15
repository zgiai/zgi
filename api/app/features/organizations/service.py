from datetime import datetime, timedelta
from typing import Optional, Annotated
from fastapi import Depends, HTTPException, status, Body
from sqlalchemy.orm import Session

from app.core.auth import get_current_user
from app.core.database import get_db
from app.features import Project, APIKey
from app.features.organizations.models import OrganizationMember
from app.features.projects.models import ProjectStatus
from app.features.users.models import User


def organization_require_admin(
        organization_id: Annotated[int, Body(embed=True)],
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_db)
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

def organization_require_member_admin(
        member_id: Annotated[int, Body(embed=True)],
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_db)
) -> User:
    """Require access"""
    member = db.query(OrganizationMember).filter(OrganizationMember.id == member_id).first()
    if not member:
        raise HTTPException(status_code=404, detail="Organization member not found")
    org_admin_member = db.query(OrganizationMember).filter(
        OrganizationMember.user_id == current_user.id,
        OrganizationMember.organization_id == member.organization_id,
        OrganizationMember.is_admin == True
    ).first()
    if not org_admin_member and not current_user.is_superuser:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Insufficient permissions. Project admin access required."
        )
    return current_user
