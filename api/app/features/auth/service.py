from datetime import datetime, timedelta
from typing import Optional, Dict, Any
from fastapi import HTTPException, status
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError
import jwt
import logging
from passlib.context import CryptContext

from app.core.config import settings
from app.features.users.models import User

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Password hashing context
pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")

class AuthService:
    def __init__(self, db: Session):
        self.db = db

    def verify_password(self, plain_password: str, hashed_password: str) -> bool:
        """Verify a password against a hash"""
        return pwd_context.verify(plain_password, hashed_password)

    def get_password_hash(self, password: str) -> str:
        """Hash a password"""
        return pwd_context.hash(password)

    def create_access_token(self, user_id: int) -> str:
        """Create JWT access token"""
        to_encode = {
            "sub": str(user_id),
            "exp": datetime.utcnow() + timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
        }
        return jwt.encode(to_encode, settings.SECRET_KEY, algorithm=settings.ALGORITHM)

    def get_current_user(self, token: str) -> User:
        """Get current user from JWT token"""
        try:
            payload = jwt.decode(token, settings.SECRET_KEY, algorithms=[settings.ALGORITHM])
            user_id = int(payload.get("sub"))
            if user_id is None:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Could not validate credentials",
                )
        except jwt.PyJWTError:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not validate credentials",
            )

        user = self.db.query(User).filter(User.id == user_id).first()
        if user is None:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="User not found",
            )
        return user

    def authenticate_user(self, email: str, password: str) -> Dict[str, Any]:
        """Authenticate user and return token"""
        user = self.db.query(User).filter(User.email == email).first()
        if not user or not self.verify_password(password, user.hashed_password):
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Incorrect email or password",
            )
        
        access_token = self.create_access_token(user.id)
        return {
            "access_token": access_token,
            "token_type": "bearer",
            "user": {
                "id": user.id,
                "email": user.email,
                "username": user.username
            }
        }

    def register_user(self, email: str, username: str, password: str) -> User:
        """Register a new user"""
        try:
            # Check if user already exists
            if self.db.query(User).filter(User.email == email).first():
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Email already registered",
                )

            if self.db.query(User).filter(User.username == username).first():
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Username already taken",
                )

            # Create new user
            user = User(
                email=email,
                username=username,
                hashed_password=self.get_password_hash(password),
                is_active=True,
                is_verified=True,
            )
            self.db.add(user)
            self.db.commit()
            self.db.refresh(user)
            return user
        except Exception as e:
            self.db.rollback()
            if isinstance(e, HTTPException):
                raise e
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="An error occurred during registration",
            )
