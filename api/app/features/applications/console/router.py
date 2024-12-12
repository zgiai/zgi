from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.orm import Session
from typing import List, Optional
from uuid import UUID

from app.core.deps import get_db
from app.core.security.auth import get_current_active_user
from app.features.users.models import User
from app.features.applications.models import Application
from app.features.applications.schemas import (
    ApplicationCreate,
    ApplicationUpdate,
    ApplicationResponse,
    ApplicationList
)

router = APIRouter()

@router.post("", response_model=ApplicationResponse)
async def create_application(
    *,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_active_user),
    application: ApplicationCreate
):
    """
    Create a new application.
    """
    # 检查用户是否有权限创建应用
    if not current_user.is_active:
        raise HTTPException(status_code=403, detail="Inactive user")

    # 创建新应用
    db_application = Application(
        name=application.name,
        description=application.description,
        owner_id=current_user.id,
        api_key_prefix=application.api_key_prefix,
        max_tokens=application.max_tokens,
        max_requests_per_day=application.max_requests_per_day,
        is_active=True
    )
    
    db.add(db_application)
    db.commit()
    db.refresh(db_application)
    
    return db_application

@router.get("", response_model=List[ApplicationList])
async def list_applications(
    *,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_active_user),
    skip: int = 0,
    limit: int = 100
):
    """
    Retrieve applications.
    """
    # 检查用户是否有权限查看应用列表
    if not current_user.is_active:
        raise HTTPException(status_code=403, detail="Inactive user")
        
    # 获取应用列表
    applications = db.query(Application).filter(
        Application.owner_id == current_user.id
    ).offset(skip).limit(limit).all()
    
    return applications

@router.get("/{application_id}", response_model=ApplicationResponse)
async def get_application(
    *,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_active_user),
    application_id: int
):
    """
    Get application by ID.
    """
    # 检查用户是否有权限查看应用
    if not current_user.is_active:
        raise HTTPException(status_code=403, detail="Inactive user")
        
    # 获取应用
    application = db.query(Application).filter(
        Application.id == application_id,
        Application.owner_id == current_user.id
    ).first()
    
    if not application:
        raise HTTPException(status_code=404, detail="Application not found")
        
    return application

@router.put("/{application_id}", response_model=ApplicationResponse)
async def update_application(
    *,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_active_user),
    application_id: int,
    application_update: ApplicationUpdate
):
    """
    Update application.
    """
    # 检查用户是否有权限更新应用
    if not current_user.is_active:
        raise HTTPException(status_code=403, detail="Inactive user")
        
    # 获取应用
    db_application = db.query(Application).filter(
        Application.id == application_id,
        Application.owner_id == current_user.id
    ).first()
    
    if not db_application:
        raise HTTPException(status_code=404, detail="Application not found")
    
    # 更新应用信息
    for field, value in application_update.dict(exclude_unset=True).items():
        setattr(db_application, field, value)
    
    db.commit()
    db.refresh(db_application)
    
    return db_application

@router.delete("/{application_id}")
async def delete_application(
    *,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_active_user),
    application_id: int
):
    """
    Delete application.
    """
    # 检查用户是否有权限删除应用
    if not current_user.is_active:
        raise HTTPException(status_code=403, detail="Inactive user")
        
    # 获取应用
    db_application = db.query(Application).filter(
        Application.id == application_id,
        Application.owner_id == current_user.id
    ).first()
    
    if not db_application:
        raise HTTPException(status_code=404, detail="Application not found")
    
    # 删除应用
    db.delete(db_application)
    db.commit()
    
    return {"detail": "Application deleted"}
