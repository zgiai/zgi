from datetime import datetime, timedelta
from typing import Optional
from fastapi import Depends, HTTPException, status
from sqlalchemy.orm import Session

from app.core.auth import get_current_user
from app.core.database import get_db, get_sync_db
from app.features.api_keys.models import APIKey
from app.features.projects.models import Project
from app.features.organizations.models import OrganizationMember
from app.features.projects.models import ProjectStatus
from app.features.users.models import User

def project_require_admin(
        project_id: int,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require project access"""
    project = db.query(Project).filter(
        Project.id == project_id,
        Project.status != ProjectStatus.DELETED
    ).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    org_admin_member = db.query(OrganizationMember).filter(
        OrganizationMember.user_id == current_user.id,
        OrganizationMember.organization_id == project.organization_id,
        OrganizationMember.is_admin == True
    ).first()
    if not org_admin_member and not current_user.is_superuser and current_user.user_type == 0:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Insufficient permissions. Project admin access required."
        )
    return current_user

def project_require_member(
        project_id: int,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require project access"""
    project = db.query(Project).filter(
        Project.id == project_id,
        Project.status != ProjectStatus.DELETED
    ).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    org_member = db.query(OrganizationMember).filter(
        OrganizationMember.user_id == current_user.id,
        OrganizationMember.organization_id == project.organization_id
    ).first()
    if not org_member and not current_user.is_superuser and current_user.user_type == 0:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Insufficient permissions. Project admin access required."
        )
    return current_user
