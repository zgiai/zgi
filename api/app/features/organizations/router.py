from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from typing import List

from app.core.database import get_db
from app.core.security.auth import get_current_user
from app.features.users.models import User
from app.features.organizations.models import Organization, OrganizationMember, OrganizationRole
from app.features.organizations import schemas

router = APIRouter(tags=["organizations"])

@router.post("/", response_model=schemas.Organization)
def create_organization(
    org_data: schemas.OrganizationCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Create a new organization"""
    org = Organization(
        name=org_data.name,
        description=org_data.description,
        created_by=current_user.id
    )
    db.add(org)
    db.flush()

    # Add creator as owner
    member = OrganizationMember(
        organization_id=org.id,
        user_id=current_user.id,
        role=OrganizationRole.OWNER
    )
    db.add(member)
    db.commit()
    db.refresh(org)
    return org

@router.get("/", response_model=List[schemas.Organization])
def list_organizations(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """List organizations for current user"""
    query = (
        db.query(Organization)
        .join(OrganizationMember)
        .filter(OrganizationMember.user_id == current_user.id)
    )
    return query.all()

@router.get("/{org_id}", response_model=schemas.Organization)
def get_organization(
    org_id: str,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Get organization details"""
    org = (
        db.query(Organization)
        .join(OrganizationMember)
        .filter(
            Organization.id == org_id,
            OrganizationMember.user_id == current_user.id
        )
        .first()
    )
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")
    return org

@router.put("/{org_id}", response_model=schemas.Organization)
def update_organization(
    org_id: str,
    org_data: schemas.OrganizationUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Update organization details"""
    org = (
        db.query(Organization)
        .join(OrganizationMember)
        .filter(
            Organization.id == org_id,
            OrganizationMember.user_id == current_user.id,
            OrganizationMember.role.in_([OrganizationRole.OWNER, OrganizationRole.ADMIN])
        )
        .first()
    )
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found or insufficient permissions")
    
    for field, value in org_data.dict(exclude_unset=True).items():
        setattr(org, field, value)
    
    db.commit()
    db.refresh(org)
    return org
