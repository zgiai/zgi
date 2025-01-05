import secrets
from typing import List, Optional
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from datetime import datetime

from app.core.database import get_db, get_sync_db
from app.core.security import get_current_user
from app.features.users.models import User
from app.features.projects.models import Project
from . import schemas, models
from .schemas import APIKeyResponse, APIKeyList
from .service import api_keys_params_require_project_admin, api_keys_params_require_project_member, api_keys_params_require_uuid_member, \
    api_keys_params_require_uuid_admin
from ...core.base import resp_200

router = APIRouter(tags=["api_keys"])

def generate_api_key() -> str:
    """Generate a secure API key"""
    return f"zgi_{secrets.token_urlsafe(32)}"

@router.post("/projects/create")
def create_api_key(
    project_id: int,
    api_key_data: schemas.APIKeyCreate,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_project_admin)
):
    """Create a new API key for a project"""
    project = db.query(Project).filter(Project.id == project_id).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    api_key = models.APIKey(
        name=api_key_data.name,
        key=generate_api_key(),
        project_id=project.id,
        created_by=current_user.id
    )

    db.add(api_key)
    db.commit()
    db.refresh(api_key)

    return resp_200(APIKeyResponse.model_validate(api_key))

@router.get("/projects/list")
def list_api_keys(
    project_id: int,
    page_size: Optional[int] = 10,
    page_num: Optional[int] = 1,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_project_member)
):
    """List all API keys for a project"""
    project = db.query(Project).filter(Project.id == project_id).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    query = db.query(models.APIKey).filter(
        models.APIKey.project_id == project.id,
        models.APIKey.status != models.APIKeyStatus.DELETED
    )
    total = query.count()
    if page_size and page_num:
        query = query.offset((page_num - 1) * page_size).limit(page_size)
    api_key_list = query.all()

    return resp_200(APIKeyList(api_keys=api_key_list, total=total))

@router.get("/projects/info")
def list_api_keys(
    api_key_uuid: str,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_uuid_member)
):
    """Get API keys"""
    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == api_key_uuid,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")

    return resp_200(APIKeyResponse.model_validate(api_key))

@router.put("/projects/update")
def update_api_key_name(
    api_key_uuid: str,
    api_key_data: schemas.APIKeyCreate,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_uuid_admin)
):
    """Update the name of an API key"""
    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == api_key_uuid,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")
    name = api_key_data.name
    if not name:
        raise HTTPException(status_code=400, detail="Name cannot be empty")
    if api_key.name == name:
        raise HTTPException(status_code=400, detail="Name cannot be the same")
    api_key.name = name
    db.commit()
    db.refresh(api_key)

    return resp_200(APIKeyResponse.model_validate(api_key))

@router.post("/projects/disable")
def disable_api_key(
    api_key_uuid: str,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_uuid_admin)
):
    """Disable an API key"""
    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == api_key_uuid,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")

    api_key.status = models.APIKeyStatus.DISABLE
    db.commit()
    db.refresh(api_key)

    return resp_200(APIKeyResponse.model_validate(api_key))

@router.post("/projects/enable")
def disable_api_key(
    api_key_uuid: str,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_uuid_admin)
):
    """Disable an API key"""
    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == api_key_uuid,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")

    api_key.status = models.APIKeyStatus.ACTIVE
    db.commit()
    db.refresh(api_key)

    return resp_200(APIKeyResponse.model_validate(api_key))

@router.delete("/projects/delete")
def delete_api_key(
    api_key_uuid: str,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(api_keys_params_require_uuid_admin)
):
    """Soft delete an API key"""
    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == api_key_uuid,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")

    api_key.status = models.APIKeyStatus.DELETED
    db.commit()

    return resp_200(message="API key deleted")
