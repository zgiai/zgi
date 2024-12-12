from typing import List
from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm
from sqlalchemy.orm import Session
import logging

from app.core.database import get_db
from app.core.auth import require_super_admin
from app.features.users.models import User
from app.features.auth.schemas import UserResponse, UserLogin, UserRegister, Token
from app.features.auth.service import AuthService

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
    result = auth_service.authenticate_user(form_data.username, form_data.password)
    return Token(
        access_token=result["access_token"],
        token_type=result["token_type"]
    )

@router.post("/login")
def login(
    user_data: UserLogin,
    auth_service: AuthService = Depends(get_auth_service)
):
    """Login endpoint"""
    return auth_service.authenticate_user(user_data.email, user_data.password)

@router.post("/register", response_model=UserResponse, status_code=status.HTTP_201_CREATED)
def register(
    user_data: UserRegister,
    auth_service: AuthService = Depends(get_auth_service)
):
    """Register a new user"""
    try:
        logger.info(f"Attempting to register user with email: {user_data.email}")
        user = auth_service.register_user(user_data.email, user_data.username, user_data.password)
        logger.info(f"Successfully registered user: {user.email}")
        return user
    except HTTPException as e:
        logger.error(f"Error registering user: {e}")
        raise
    except Exception as e:
        logger.error(f"Error registering user: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )

@router.get("/users", response_model=List[UserResponse])
def list_users(
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """List all users (Admin only)"""
    users = db.query(User).all()
    return users

@router.get("/users/{user_id}", response_model=UserResponse)
def get_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Get user details (Admin only)"""
    user = db.query(User).filter(User.id == user_id).first()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    return user

@router.delete("/users/{user_id}")
def delete_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Delete user (Admin only)"""
    try:
        # 不允许删除当前用户
        if user_id == current_user.id:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Cannot delete current user"
            )
        
        user = db.query(User).filter(User.id == user_id).first()
        if not user:
            raise HTTPException(status_code=404, detail="User not found")

        # 删除用户相关的所有数据
        db.query(User).filter(User.id == user_id).delete()
        db.commit()

        return {"message": "User deleted successfully"}
    except Exception as e:
        db.rollback()
        logger.error(f"Error deleting user: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Error deleting user: {str(e)}"
        )

@router.get("/me")
def get_current_user_info(current_user: User = Depends(get_current_user)):
    """Get current user information"""
    return {
        "id": current_user.id,
        "email": current_user.email,
        "username": current_user.username,
        "is_active": current_user.is_active,
        "is_verified": current_user.is_verified,
        "is_superuser": current_user.is_superuser
    }
