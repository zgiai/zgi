from typing import List, Dict, Any, Optional, Annotated
from fastapi import APIRouter, Depends, HTTPException, status, Query
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm
from sqlalchemy.orm import Session
import logging

from app.core.base import resp_200
from app.core.database import get_sync_db
from app.core.auth import require_super_admin
from app.core.init_data import init_default_organization_data
from app.features.auth.service import AuthService, get_auth_service
from app.features.auth.schemas import (
    Token,
    UserCreate,
    UserLogin,
    UserResponse,
    UserList
)
from app.features.organizations.models import OrganizationMember
from app.features.users.models import User

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

router = APIRouter(prefix="/v1", tags=["auth"])
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/token")

def get_auth_service(db: Session = Depends(get_sync_db)) -> AuthService:
    return AuthService(db)

def get_current_user(
    token: str = Depends(oauth2_scheme),
    auth_service: AuthService = Depends(get_auth_service)
) -> User:
    return auth_service.get_current_user(token)

@router.post("/token", response_model=Token)
async def login_for_access_token(
    form_data: OAuth2PasswordRequestForm = Depends(),
    auth_service: AuthService = Depends(get_auth_service)
):
    """Login endpoint to get access token"""
    try:
        result = auth_service.authenticate_user(form_data.username, form_data.password)
        return Token(
            access_token=result["access_token"],
            token_type=result["token_type"]
        )
    except HTTPException as e:
        logger.error(f"Login failed: {e.detail}")
        raise

@router.post("/login")
def login(
    user_data: UserLogin,
    auth_service: AuthService = Depends(get_auth_service)
):
    """Login endpoint"""
    try:
        auth_data = auth_service.authenticate_user(user_data.email, user_data.password)
        return resp_200(data=auth_data)
    except HTTPException as e:
        logger.error(f"Login failed: {e.detail}")
        raise

@router.post("/register", status_code=status.HTTP_201_CREATED)
def register(
    user_data: UserCreate,
    auth_service: AuthService = Depends(get_auth_service),
    db: Session = Depends(get_sync_db)
):
    """Register a new user"""
    try:
        # Check if first user, create admin account
        admin = db.query(User).filter(User.id == 1).first()
        if not admin:
            user = auth_service.register_admin(
                email=user_data.email,
                username=user_data.username,
                password=user_data.password
            )
            init_default_organization_data(user.id)
        else:
            user = auth_service.register_user(
                email=user_data.email,
                username=user_data.username,
                password=user_data.password
            )
        return resp_200(data=UserResponse.model_validate(user))
    except HTTPException as e:
        if "Email already registered" in str(e.detail):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Email already registered"
            )
        elif "Username already taken" in str(e.detail):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Username already taken"
            )
        raise

@router.get("/me")
def get_current_user_info(current_user: User = Depends(get_current_user)):
    """Get current user information"""
    return resp_200(UserResponse.model_validate(current_user))

@router.get("/users")
def list_users(
               page_size: Optional[int] = 10,
               page_num: Optional[int] = 1,
               org_id: Annotated[List[int], Query()] = None,
               role_id: Annotated[List[int], Query()] = None,
               current_user: User = Depends(require_super_admin),
               db: Session = Depends(get_sync_db)
):
    """List all users (admin only)"""
    query = db.query(User)
    if org_id:
        query = query.filter(User.organization_members.any(OrganizationMember.organization_id.in_(org_id)))
    if role_id:
        query = query.filter(User.organization_members.any(OrganizationMember.role_id.in_(role_id)))
    total = query.count()
    if page_size and page_num:
        query = query.offset((page_num - 1) * page_size).limit(page_size)
    users = query.all()
    return resp_200(data=UserList(users=users, total=total))

@router.get("/users/{user_id}")
def get_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_sync_db)
):
    """Get user by ID (admin only)"""
    user = db.query(User).filter(User.id == user_id).first()
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    return resp_200(UserResponse.model_validate(user))

@router.delete("/users/{user_id}")
def delete_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    auth_service: AuthService = Depends(get_auth_service)
):
    """Delete user (Admin only)"""
    try:
        # Don't allow deleting current user
        if user_id == current_user.id:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Cannot delete current user"
            )
        
        # Use auth service to delete user
        deleted_user = auth_service.delete_user(user_id, current_user)
        return resp_200(data={"id": deleted_user.id}, message="User deleted successfully")
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error deleting user: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Error deleting user: {str(e)}"
        )
