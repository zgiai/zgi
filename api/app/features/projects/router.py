from typing import List, Optional
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from sqlalchemy.exc import SQLAlchemyError

from app.core.database import get_sync_db
from app.core.security import get_current_user
from app.features.users.models import User
from app.features.organizations.models import Organization
from . import schemas, models
from .schemas import ProjectResponse, ProjectList
from .service import project_require_admin
from .. import APIKey
from ..organizations.service import organization_body_require_admin
from ...core.api_key import generate_api_key
from ...core.base import resp_200

router = APIRouter(tags=["projects"])

@router.post("/create")
def create_project(
    project_data: schemas.ProjectCreate,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_body_require_admin)
):
    """Create a new project"""
    # Get organization by id
    org = db.query(Organization).filter(Organization.id == project_data.organization_id).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")

    # Create project
    project = models.Project(
        name=project_data.name,
        description=project_data.description,
        organization_id=org.id,
        created_by=current_user.id,
        status=project_data.status
    )
    
    try:
        db.add(project)
        db.commit()
        db.refresh(project)

        # create default api key
        api_key = APIKey(name="Default API Key", key=generate_api_key(), created_by=current_user.id,
                         project_id=project.id)
        db.add(api_key)
        db.commit()

        return resp_200(ProjectResponse.model_validate(project))
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))

@router.get("/list")
def list_projects(
    organization_id: int,
    page_size: Optional[int] = 10,
    page_num: Optional[int] = 1,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user)
):
    """List all projects for an organization"""
    org = db.query(Organization).filter(Organization.id == organization_id).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")
    query = db.query(models.Project).filter(
        models.Project.organization_id == org.id,
        models.Project.status != models.ProjectStatus.DELETED
    )
    total = query.count()
    if page_size and page_num:
        query = query.offset((page_num - 1) * page_size).limit(page_size)
    project_list = query.all()
    
    return resp_200(data=ProjectList(projects=project_list, total=total))

@router.get("/info")
def get_project(
    project_id: int,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user)
):
    """Get a specific project by UUID"""
    project = db.query(models.Project).filter(
        models.Project.id == project_id,
        models.Project.status != models.ProjectStatus.DELETED
    ).first()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    return resp_200(ProjectResponse.model_validate(project))

@router.put("/update")
def update_project(
    project_id: int,
    project_data: schemas.ProjectUpdate,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(project_require_admin)
):
    """Update a project"""
    project = db.query(models.Project).filter(
        models.Project.id == project_id,
        models.Project.status != models.ProjectStatus.DELETED
    ).first()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    # Update project fields
    update_data = project_data.model_dump(exclude_unset=True)
    for field, value in update_data.items():
        if value is not None and hasattr(project, field):
            setattr(project, field, value)

    try:
        db.commit()
        db.refresh(project)
        return resp_200(ProjectResponse.model_validate(project))
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))

@router.delete("/delete")
def delete_project(
    project_id: int,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(project_require_admin)
):
    """Soft delete a project"""
    project = db.query(models.Project).filter(
        models.Project.id == project_id,
        models.Project.status != models.ProjectStatus.DELETED
    ).first()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    # Soft delete the project
    project.status = models.ProjectStatus.DELETED

    try:
        db.commit()
        db.refresh(project)
        return resp_200(ProjectResponse.model_validate(project))
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))
