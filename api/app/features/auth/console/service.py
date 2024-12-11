from datetime import datetime, timedelta
from typing import List, Optional, Union, Dict
from sqlalchemy.orm import Session
from sqlalchemy import select
from fastapi import Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer
from jose import JWTError, jwt
from passlib.context import CryptContext
import logging
import traceback

from app.core.config import settings
from app.core.security import verify_password, create_access_token, get_password_hash
from app.models import User
from app.features.auth.console.schemas import UserResponse

logger = logging.getLogger(__name__)

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

    def register_user(self, email: str, username: str, password: str) -> User:
        """Register a new user"""
        try:
            # Check if user already exists
            logger.info(f"Checking if user with email {email} already exists")
            if self.db.query(User).filter(User.email == email).first():
                logger.warning(f"User with email {email} already exists")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Email already registered"
                )
            
            logger.info(f"Checking if username {username} already exists")
            if self.db.query(User).filter(User.username == username).first():
                logger.warning(f"Username {username} already taken")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Username already taken"
                )
            
            # Validate password
            logger.info("Validating password")
            if len(password) < 8:
                logger.warning("Password too short")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Password must be at least 8 characters long"
                )
            if not any(c.isupper() for c in password):
                logger.warning("Password missing uppercase letter")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Password must contain at least one uppercase letter"
                )
            if not any(c.islower() for c in password):
                logger.warning("Password missing lowercase letter")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Password must contain at least one lowercase letter"
                )
            if not any(c.isdigit() for c in password):
                logger.warning("Password missing number")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Password must contain at least one number"
                )
            if not any(c in "!@#$%^&*()_+-=[]{}|;:,.<>?" for c in password):
                logger.warning("Password missing special character")
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Password must contain at least one special character"
                )
            
            # Create new user
            logger.info("Creating new user")
            hashed_password = get_password_hash(password)
            user = User(
                email=email,
                username=username,
                hashed_password=hashed_password,
                is_active=True,
                is_superuser=False
            )
            logger.info("Adding user to database")
            self.db.add(user)
            logger.info("Committing transaction")
            self.db.commit()
            logger.info("Refreshing user object")
            self.db.refresh(user)
            logger.info(f"Successfully created user with email {email}")
            return user
        except HTTPException as e:
            logger.error(f"HTTP error during user registration: {e.detail}")
            raise e
        except Exception as e:
            logger.error(f"Error registering user: {str(e)}")
            logger.error(f"Error type: {type(e)}")
            logger.error(f"Error traceback: {traceback.format_exc()}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=f"Database error: {str(e)}"
            )
