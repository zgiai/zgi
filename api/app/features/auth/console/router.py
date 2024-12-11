from typing import List, Optional
from fastapi import APIRouter, Depends, HTTPException, status, Response
from sqlalchemy.orm import Session
import logging

from app.core.database import get_db
from app.core.auth import get_current_user, require_super_admin
from app.models import User
from app.features.auth.console.schemas import UserResponse, UserLogin, UserRegister
from app.features.auth.console.service import AuthConsoleService

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

router = APIRouter(prefix="/v1/console/auth", tags=["console-auth"])

@router.post("/login")
def login(
    user_data: UserLogin,
    db: Session = Depends(get_db)
):
    """Console login endpoint"""
    auth_service = AuthConsoleService(db)
    return auth_service.authenticate_user(user_data.email, user_data.password)

@router.post("/register", response_model=UserResponse, status_code=status.HTTP_201_CREATED)
def register(
    user_data: UserRegister,
    db: Session = Depends(get_db)
):
    """Register a new user"""
    try:
        logger.info(f"Attempting to register user with email: {user_data.email}")
        auth_service = AuthConsoleService(db)
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
    skip: int = 0,
    limit: int = 100,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """List all users (Admin only)"""
    auth_service = AuthConsoleService(db)
    return auth_service.list_users(skip, limit)

@router.get("/users/{user_id}", response_model=UserResponse)
def get_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Get user details (Admin only)"""
    auth_service = AuthConsoleService(db)
    user = auth_service.get_user(user_id)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    return user

@router.delete("/users/{user_id}")
def delete_user(
    user_id: int,
    current_user: User = Depends(require_super_admin),
    db: Session = Depends(get_db)
):
    """Delete user (Admin only)"""
    auth_service = AuthConsoleService(db)
    success = auth_service.delete_user(user_id)
    if not success:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    return {"message": "User deleted successfully"}

@router.get("/protected-resource")
def protected_resource(current_user: User = Depends(get_current_user)):
    """Protected resource endpoint for testing"""
    return {
        "message": "You have access to this protected resource",
        "user_id": current_user.id
    }
