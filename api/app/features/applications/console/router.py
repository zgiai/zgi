from typing import List, Optional
from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy.orm import Session
from sqlalchemy import or_

from app.core.database import get_db
from app.core.auth import get_current_user
from app.features.applications import schemas
from app.features.applications.models import Application
from app.features.teams.models import Team, TeamMember
from app.features.users.models import User

router = APIRouter(prefix="/v1/console/applications", tags=["applications"])

@router.post("", response_model=schemas.Application)
def create_application(
    application: schemas.ApplicationCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """创建新应用"""
    # 检查团队权限（如果是团队应用）
    if application.team_id:
        team = db.query(Team).filter(Team.id == application.team_id).first()
        if not team:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Team not found"
            )
        
        # 检查用户是否是团队成员
        member = (
            db.query(TeamMember)
            .filter(
                TeamMember.team_id == application.team_id,
                TeamMember.user_id == current_user.id,
                TeamMember.role.in_(['owner', 'admin'])
            )
            .first()
        )
        if not member:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Only team owner or admin can create team applications"
            )
    
    # 创建新应用
    db_application = Application(
        name=application.name,
        description=application.description,
        type=application.type.value,
        access_level=application.access_level.value,
        team_id=application.team_id,
        created_by=current_user.id
    )
    
    db.add(db_application)
    db.commit()
    db.refresh(db_application)
    
    return db_application

@router.get("", response_model=List[schemas.Application])
def list_applications(
    search: Optional[str] = None,
    type: Optional[schemas.ApplicationType] = None,
    team_id: Optional[int] = None,
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取应用列表"""
    query = db.query(Application)
    
    # 基本过滤：用户创建的应用或者有权访问的团队应用
    team_ids = (
        db.query(TeamMember.team_id)
        .filter(TeamMember.user_id == current_user.id)
        .all()
    )
    team_ids = [team_id for (team_id,) in team_ids]
    
    query = query.filter(
        or_(
            Application.created_by == current_user.id,
            Application.team_id.in_(team_ids)
        )
    )
    
    # 搜索过滤
    if search:
        query = query.filter(
            or_(
                Application.name.ilike(f"%{search}%"),
                Application.description.ilike(f"%{search}%")
            )
        )
    
    # 类型过滤
    if type:
        query = query.filter(Application.type == type.value)
    
    # 团队过滤
    if team_id:
        if team_id not in team_ids and not current_user.is_superuser:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Not a member of this team"
            )
        query = query.filter(Application.team_id == team_id)
    
    # 分页
    total = query.count()
    applications = query.offset(skip).limit(limit).all()
    
    return applications

@router.get("/{application_id}", response_model=schemas.ApplicationDetail)
def get_application(
    application_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取应用详情"""
    application = db.query(Application).filter(Application.id == application_id).first()
    if not application:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Application not found"
        )
    
    # 检查访问权限
    if application.access_level == "private" and application.created_by != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="No permission to access this application"
        )
    
    if application.access_level == "team" and application.team_id:
        member = (
            db.query(TeamMember)
            .filter(
                TeamMember.team_id == application.team_id,
                TeamMember.user_id == current_user.id
            )
            .first()
        )
        if not member:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="No permission to access this team application"
            )
    
    return application

@router.put("/{application_id}", response_model=schemas.Application)
def update_application(
    application_id: int,
    application_update: schemas.ApplicationUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """更新应用信息"""
    application = db.query(Application).filter(Application.id == application_id).first()
    if not application:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Application not found"
        )
    
    # 检查更新权限
    if application.created_by != current_user.id:
        if application.team_id:
            member = (
                db.query(TeamMember)
                .filter(
                    TeamMember.team_id == application.team_id,
                    TeamMember.user_id == current_user.id,
                    TeamMember.role.in_(['owner', 'admin'])
                )
                .first()
            )
            if not member:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Only application creator or team admin can update application"
                )
        else:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Only application creator can update application"
            )
    
    # 如果要更改团队，检查新团队的权限
    if application_update.team_id is not None and application_update.team_id != application.team_id:
        if application_update.team_id:
            member = (
                db.query(TeamMember)
                .filter(
                    TeamMember.team_id == application_update.team_id,
                    TeamMember.user_id == current_user.id,
                    TeamMember.role.in_(['owner', 'admin'])
                )
                .first()
            )
            if not member:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="No permission to move application to this team"
                )
    
    # 更新应用信息
    update_data = application_update.dict(exclude_unset=True)
    if 'type' in update_data:
        update_data['type'] = update_data['type'].value
    if 'access_level' in update_data:
        update_data['access_level'] = update_data['access_level'].value
    
    for field, value in update_data.items():
        setattr(application, field, value)
    
    db.commit()
    db.refresh(application)
    return application

@router.delete("/{application_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_application(
    application_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """删除应用"""
    application = db.query(Application).filter(Application.id == application_id).first()
    if not application:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Application not found"
        )
    
    # 检查删除权限
    if application.created_by != current_user.id:
        if application.team_id:
            member = (
                db.query(TeamMember)
                .filter(
                    TeamMember.team_id == application.team_id,
                    TeamMember.user_id == current_user.id,
                    TeamMember.role == 'owner'
                )
                .first()
            )
            if not member:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Only application creator or team owner can delete application"
                )
        else:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Only application creator can delete application"
            )
    
    db.delete(application)
    db.commit()
    return {"message": "Application deleted successfully"}
