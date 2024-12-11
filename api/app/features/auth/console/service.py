from datetime import datetime, timedelta
from typing import List, Optional, Union, Dict
from sqlalchemy.orm import Session
from sqlalchemy import select
from fastapi import Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer
from jose import JWTError, jwt
from passlib.context import CryptContext

from app.core.config import settings
from app.core.security import verify_password, create_access_token
from app.models import User
from app.features.auth.console.schemas import UserResponse

class AuthConsoleService:
    def __init__(self, db: Session):
        self.db = db

    def authenticate_user(self, email: str, password: str) -> Dict[str, str]:
        """Authenticate user and return access token"""
        user = self.db.query(User).filter(User.email == email).first()
        if not user or not verify_password(password, user.hashed_password):
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Incorrect email or password"
            )
        
        if not user.is_superuser:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="User is not authorized for console access"
            )

        access_token = create_access_token(
            data={"sub": str(user.id)},
            expires_delta=timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
        )
        return {
            "access_token": access_token,
            "token_type": "bearer"
        }

    def list_users(self, skip: int = 0, limit: int = 100) -> List[User]:
        """List all users"""
        return self.db.query(User).offset(skip).limit(limit).all()

    def get_user(self, user_id: int) -> Optional[User]:
        """Get user by ID"""
        return self.db.query(User).filter(User.id == user_id).first()

    def delete_user(self, user_id: int) -> bool:
        """Delete user by ID"""
        user = self.get_user(user_id)
        if user:
            self.db.delete(user)
            self.db.commit()
            return True
        return False
