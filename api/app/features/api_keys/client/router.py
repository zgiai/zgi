from typing import List
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.auth import get_current_user
from app.models import User
from app.features.api_keys.client.schemas import APIKeyCreate, APIKeyUpdate, APIKeyResponse, APIKeyListResponse
from app.features.api_keys.client.service import APIKeyClientService

router = APIRouter(prefix="/v1/api-keys", tags=["api-keys"])

@router.post("", response_model=APIKeyResponse)
def create_api_key(
    api_key_data: APIKeyCreate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """创建新的 API 密钥"""
    service = APIKeyClientService(db)
    return service.create_api_key(current_user.id, api_key_data)

@router.get("", response_model=List[APIKeyListResponse])
def list_api_keys(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取当前用户的所有 API 密钥列表"""
    service = APIKeyClientService(db)
    return service.get_user_api_keys(current_user.id)

@router.get("/{api_key_id}", response_model=APIKeyResponse)
def get_api_key(
    api_key_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """获取特定的 API 密钥详情"""
    service = APIKeyClientService(db)
    api_key = service.get_api_key(current_user.id, api_key_id)
    if not api_key:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="API key not found"
        )
    return api_key

@router.put("/{api_key_id}", response_model=APIKeyResponse)
def update_api_key(
    api_key_id: int,
    api_key_data: APIKeyUpdate,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """更新 API 密钥"""
    service = APIKeyClientService(db)
    return service.update_api_key(current_user.id, api_key_id, api_key_data)

@router.delete("/{api_key_id}")
def delete_api_key(
    api_key_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """删除 API 密钥"""
    service = APIKeyClientService(db)
    service.delete_api_key(current_user.id, api_key_id)
    return {"message": "API key deleted successfully"}
