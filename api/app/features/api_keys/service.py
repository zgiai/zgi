from datetime import datetime, timedelta
from typing import Optional
from fastapi import Depends, HTTPException, status, Request
from pydantic import ValidationError
from sqlalchemy.orm import Session

from app.core.auth import get_current_user
from app.core.database import get_db, get_sync_db
from app.features.api_keys.models import APIKey
from app.features.projects.models import Project
from app.features.organizations.models import OrganizationMember
from app.features.projects.models import ProjectStatus
from app.features.users.models import User

async def extract_body_project_id(request: Request) -> int:
    try:
        json_data = await request.json()
        project_id = json_data.get("project_id")
        return project_id
    except (ValidationError, KeyError) as e:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=str(e)
        )

def api_keys_params_require_project_admin(
        project_id: int,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require project access"""
    project = db.query(Project).filter(
        Project.id == project_id
    ).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    if project.status == ProjectStatus.DELETED:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Project is archived or deleted"
        )
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

def api_keys_body_require_project_admin(
        project_id: int = Depends(extract_body_project_id),
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    return api_keys_params_require_project_admin(project_id, current_user, db)

def api_keys_params_require_project_member(
        project_id: int,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require project access"""
    project = db.query(Project).filter(
        Project.id == project_id
    ).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    if project.status == ProjectStatus.DELETED:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Project is archived or deleted"
        )
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

def api_keys_params_require_uuid_admin(
        api_key_uuid: str,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require api_key access"""
    api_key = db.query(APIKey).filter(
        APIKey.uuid == api_key_uuid,
        Project.status != ProjectStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="api_key not found")
    return api_keys_params_require_project_admin(api_key.project_id, current_user, db)

def api_keys_params_require_uuid_member(
        api_key_uuid: str,
        current_user: User = Depends(get_current_user),
        db: Session = Depends(get_sync_db)
) -> User:
    """Require api_key access"""
    api_key = db.query(APIKey).filter(
        APIKey.uuid == api_key_uuid,
        Project.status != ProjectStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="api_key not found")
    return api_keys_params_require_project_member(api_key.project_id, current_user, db)
