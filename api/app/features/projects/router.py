from typing import List
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from sqlalchemy.exc import SQLAlchemyError

from app.core.database import get_db
from app.core.security import get_current_user
from app.features.users.models import User
from app.features.organizations.models import Organization
from . import schemas, models

router = APIRouter(tags=["projects"])

@router.post("", response_model=schemas.Project)
def create_project(
    project_data: schemas.ProjectCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Create a new project"""
    # Get organization by UUID
    org = db.query(Organization).filter(Organization.uuid == project_data.organization_uuid).first()
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
        # Get organization for response
        org = db.query(Organization).filter(Organization.id == project.organization_id).first()
        return schemas.Project(
            uuid=project.uuid,
            name=project.name,
            description=project.description,
            organization_uuid=org.uuid,
            created_by=project.created_by,
            status=project.status,
            created_at=project.created_at,
            updated_at=project.updated_at
        )
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))

@router.get("", response_model=List[schemas.Project])
def list_projects(
    organization_uuid: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """List all projects for an organization"""
    org = db.query(Organization).filter(Organization.uuid == organization_uuid).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")

    projects = db.query(models.Project).filter(
        models.Project.organization_id == org.id,
        models.Project.status != models.ProjectStatus.DELETED
    ).all()
    
    return [schemas.Project(
        uuid=p.uuid,
        name=p.name,
        description=p.description,
        organization_uuid=org.uuid,
        created_by=p.created_by,
        status=p.status,
        created_at=p.created_at,
        updated_at=p.updated_at
    ) for p in projects]

@router.get("/{project_uuid}", response_model=schemas.Project)
def get_project(
    project_uuid: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get a specific project by UUID"""
    project = db.query(models.Project).filter(
        models.Project.uuid == project_uuid,
        models.Project.status != models.ProjectStatus.DELETED
    ).first()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    # Get organization for response
    org = db.query(Organization).filter(Organization.id == project.organization_id).first()
    return schemas.Project(
        uuid=project.uuid,
        name=project.name,
        description=project.description,
        organization_uuid=org.uuid,
        created_by=project.created_by,
        status=project.status,
        created_at=project.created_at,
        updated_at=project.updated_at
    )

@router.put("/{project_uuid}", response_model=schemas.Project)
def update_project(
    project_uuid: str,
    project_data: schemas.ProjectUpdate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Update a project"""
    project = db.query(models.Project).filter(
        models.Project.uuid == project_uuid,
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
        # Get organization for response
        org = db.query(Organization).filter(Organization.id == project.organization_id).first()
        return schemas.Project(
            uuid=project.uuid,
            name=project.name,
            description=project.description,
            organization_uuid=org.uuid,
            created_by=project.created_by,
            status=project.status,
            created_at=project.created_at,
            updated_at=project.updated_at
        )
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))

@router.delete("/{project_uuid}", response_model=schemas.Project)
def delete_project(
    project_uuid: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Soft delete a project"""
    project = db.query(models.Project).filter(
        models.Project.uuid == project_uuid,
        models.Project.status != models.ProjectStatus.DELETED
    ).first()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    # Soft delete the project
    project.status = models.ProjectStatus.DELETED

    try:
        db.commit()
        db.refresh(project)
        # Get organization for response
        org = db.query(Organization).filter(Organization.id == project.organization_id).first()
        return schemas.Project(
            uuid=project.uuid,
            name=project.name,
            description=project.description,
            organization_uuid=org.uuid,
            created_by=project.created_by,
            status=project.status,
            created_at=project.created_at,
            updated_at=project.updated_at
        )
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))
