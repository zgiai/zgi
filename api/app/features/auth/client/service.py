from datetime import datetime, timedelta
from typing import Optional, Dict, Any
from fastapi import HTTPException, status
from jose import JWTError, jwt
from passlib.context import CryptContext
from sqlalchemy.orm import Session
import logging
import time

from app.models import User
from app.core.config import settings

logger = logging.getLogger(__name__)

class AuthClientService:
    """Service for handling authentication operations"""
    pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")
    
    def __init__(self, db: Session):
        self.db = db
    
    def get_user_by_email(self, email: str) -> Optional[User]:
        """Get a user by email"""
        return self.db.query(User).filter(User.email == email).first()

    def get_user_by_id(self, user_id: int) -> Optional[User]:
        """Get a user by their ID"""
        return self.db.query(User).filter(User.id == user_id).first()

    def verify_password(self, plain_password: str, hashed_password: str) -> bool:
        """Verify a password against its hash"""
        return self.pwd_context.verify(plain_password, hashed_password)

    def get_password_hash(self, password: str) -> str:
        """Hash a password"""
        return self.pwd_context.hash(password)

    def create_access_token(
        self,
        data: Dict[str, Any],
        expires_delta: Optional[timedelta] = None
    ) -> str:
        """Create a JWT access token"""
        to_encode = data.copy()
        if expires_delta:
            expire = datetime.utcnow() + expires_delta
        else:
            expire = datetime.utcnow() + timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
        to_encode.update({
            "exp": expire.timestamp(),  # Use timestamp() for expiration
            "nonce": str(time.time())  # Add a nonce to ensure uniqueness
        })
        encoded_jwt = jwt.encode(to_encode, settings.SECRET_KEY, algorithm=settings.ALGORITHM)
        return encoded_jwt

    def verify_token(self, token: str) -> Dict[str, Any]:
        """Verify a JWT token"""
        try:
            # First decode without verification to get the expiration time
            unverified_payload = jwt.get_unverified_claims(token)
            exp_timestamp = unverified_payload.get("exp")
            
            # Check if token has expired
            if exp_timestamp and time.time() > exp_timestamp:
                raise jwt.ExpiredSignatureError()
            
            # Now verify the token
            payload = jwt.decode(
                token,
                settings.SECRET_KEY,
                algorithms=[settings.ALGORITHM]
            )
            return payload
        except jwt.ExpiredSignatureError:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Token has expired"
            )
        except jwt.JWTError:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not validate credentials"
            )

    def get_current_user(self, token: str) -> User:
        """Get the current user from a JWT token"""
        try:
            payload = self.verify_token(token)
            email = payload.get("sub")
            if email is None:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Could not validate credentials"
                )
            user = self.get_user_by_email(email)
            if user is None:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="User not found"
                )
            return user
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error in get_current_user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not validate credentials"
            )

    def validate_password(self, password: str) -> None:
        """
        Validate password strength
        
        Args:
            password: Password to validate
            
        Raises:
            HTTPException: If password doesn't meet requirements
        """
        if not password:
            raise HTTPException(
                status_code=400,
                detail="Password cannot be empty"
            )
        
        if len(password) < 8:
            raise HTTPException(
                status_code=400,
                detail="Password must be at least 8 characters long"
            )
        
        if not any(char.isdigit() for char in password):
            raise HTTPException(
                status_code=400,
                detail="Password must contain at least one number"
            )
        
        if not any(char.isalpha() for char in password):
            raise HTTPException(
                status_code=400,
                detail="Password must contain at least one letter"
            )

    def login(self, email: str, password: str) -> Dict[str, str]:
        """Authenticate a user and return an access token"""
        try:
            user = self.get_user_by_email(email)
            if not user:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Incorrect email or password"
                )
            
            if not user.is_active:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Inactive user"
                )
                
            if not self.verify_password(password, user.hashed_password):
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Incorrect email or password"
                )

            access_token = self.create_access_token(data={"sub": user.email})
            return {"access_token": access_token, "token_type": "bearer"}
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error in login: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not validate credentials"
            )

    def refresh_token(self, token: str) -> str:
        """Create a new token with a new expiration time"""
        try:
            # Verify the old token is valid
            payload = self.verify_token(token)
            email = payload.get("sub")
            if email is None:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Could not validate credentials"
                )
            
            # Create new token with a forced new expiration time
            return self.create_access_token(
                data={"sub": email},
                expires_delta=timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
            )
        except Exception as e:
            logger.error(f"Error in refresh_token: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not refresh token"
            )

    def register_user(self, email: str, username: str, password: str) -> User:
        """Register a new user"""
        try:
            # Validate password
            self.validate_password(password)
            
            # Check if email already exists
            if self.get_user_by_email(email):
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Email already registered"
                )
            
            # Check if username already exists
            if self.db.query(User).filter(User.username == username).first():
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Username already taken"
                )
            
            # Create new user
            user = User(
                email=email,
                username=username,
                hashed_password=self.get_password_hash(password),
                is_active=True,
                is_superuser=False
            )
            self.db.add(user)
            self.db.commit()
            self.db.refresh(user)
            return user
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error in register_user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Could not register user"
            )

    def change_password(self, user: User, old_password: str, new_password: str) -> None:
        """Change a user's password"""
        try:
            # Verify old password
            if not self.verify_password(old_password, user.hashed_password):
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Incorrect password"
                )
            
            # Validate new password
            self.validate_password(new_password)
            
            # Update password
            user.hashed_password = self.get_password_hash(new_password)
            self.db.commit()
        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error in change_password: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Could not change password"
            )
