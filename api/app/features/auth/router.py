from typing import List, Dict, Any
from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm
from sqlalchemy.orm import Session
import logging

from app.core.database import get_db
from app.core.auth import require_super_admin
from app.features.auth.service import AuthService, get_auth_service
from app.features.auth.schemas import (
    Token,
    UserCreate,
    UserLogin,
    UserResponse,
    UserList
)
from app.features.users.models import User

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

router = APIRouter(prefix="/v1", tags=["auth"])
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/token")

def get_auth_service(db: Session = Depends(get_db)) -> AuthService:
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

@router.post("/login", response_model=Dict[str, Any])
def login(
    user_data: UserLogin,
    auth_service: AuthService = Depends(get_auth_service)
):
    """Login endpoint"""
    try:
        return auth_service.authenticate_user(user_data.email, user_data.password)
    except HTTPException as e:
        logger.error(f"Login failed: {e.detail}")
        raise

@router.post("/register", response_model=UserResponse, status_code=status.HTTP_201_CREATED)
def register(
    user_data: UserCreate,
    auth_service: AuthService = Depends(get_auth_service)
):
    """Register a new user"""
    try:
        user = auth_service.register_user(
            email=user_data.email,
            username=user_data.username,
            password=user_data.password
        )
        return user
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

@router.get("/me", response_model=UserResponse)
def get_current_user_info(current_user: User = Depends(get_current_user)):
    """Get current user information"""
    return current_user

@router.get("/users", response_model=List[UserResponse])
def list_users(
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """List all users (admin only)"""
    users = db.query(User).all()
    return users

@router.get("/users/{user_id}", response_model=UserResponse)
def get_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Get user by ID (admin only)"""
    user = db.query(User).filter(User.id == user_id).first()
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    return user

@router.delete("/users/{user_id}", response_model=Dict[str, Any])
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
        return {"message": "User deleted successfully", "id": deleted_user.id}
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error deleting user: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Error deleting user: {str(e)}"
        )
