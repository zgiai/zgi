import secrets
from typing import List
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from datetime import datetime

from app.core.database import get_db
from app.core.security import get_current_user
from app.features.users.models import User
from app.features.projects.models import Project
from . import schemas, models

router = APIRouter(tags=["api_keys"])

def generate_api_key() -> str:
    """Generate a secure API key"""
    return f"zgi_{secrets.token_urlsafe(32)}"

@router.post("/projects/{project_uuid}", response_model=schemas.APIKey)
def create_api_key(
    project_uuid: str,
    api_key_data: schemas.APIKeyCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Create a new API key for a project"""
    project = db.query(Project).filter(Project.uuid == project_uuid).first()
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

    return schemas.APIKey(
        uuid=api_key.uuid,
        name=api_key.name,
        key=api_key.key,
        project_uuid=project.uuid,
        created_by=api_key.created_by,
        status=api_key.status,
        created_at=api_key.created_at,
        updated_at=api_key.updated_at
    )

@router.get("/projects/{project_uuid}", response_model=List[schemas.APIKey])
def list_api_keys(
    project_uuid: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """List all API keys for a project"""
    project = db.query(Project).filter(Project.uuid == project_uuid).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    api_keys = db.query(models.APIKey).filter(
        models.APIKey.project_id == project.id,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).all()

    return [
        schemas.APIKey(
            uuid=key.uuid,
            name=key.name,
            key=key.key,
            project_uuid=project.uuid,
            created_by=key.created_by,
            status=key.status,
            created_at=key.created_at,
            updated_at=key.updated_at
        ) for key in api_keys
    ]

@router.post("/projects/{project_uuid}/{key_uuid}/disable")
def disable_api_key(
    project_uuid: str,
    key_uuid: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Disable an API key"""
    project = db.query(Project).filter(Project.uuid == project_uuid).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == key_uuid,
        models.APIKey.project_id == project.id,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")

    api_key.status = models.APIKeyStatus.DISABLE
    db.commit()
    db.refresh(api_key)

    return {"message": "API key disabled successfully"}

@router.delete("/projects/{project_uuid}/{key_uuid}")
def delete_api_key(
    project_uuid: str,
    key_uuid: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Soft delete an API key"""
    project = db.query(Project).filter(Project.uuid == project_uuid).first()
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    api_key = db.query(models.APIKey).filter(
        models.APIKey.uuid == key_uuid,
        models.APIKey.project_id == project.id,
        models.APIKey.status != models.APIKeyStatus.DELETED
    ).first()
    if not api_key:
        raise HTTPException(status_code=404, detail="API key not found")

    api_key.status = models.APIKeyStatus.DELETED
    db.commit()

    return {"message": "API key deleted successfully"}
