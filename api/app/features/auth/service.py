from datetime import datetime, timedelta
from typing import Optional, Dict, Any
from fastapi import HTTPException, status, Depends
from sqlalchemy.orm import Session
from sqlalchemy.exc import IntegrityError, SQLAlchemyError
import jwt
from passlib.context import CryptContext
import logging

from app.core.config import settings
from app.core.database import get_db
from app.features.users.models import User

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Password hashing context
pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")

def get_db():
    # Your database connection logic here
    pass

def get_auth_service(db: Session = Depends(get_db)) -> "AuthService":
    """Dependency to get AuthService instance"""
    return AuthService(db)

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
        try:
            expires_delta = timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
            expire = datetime.utcnow() + expires_delta
            
            to_encode = {
                "sub": str(user_id),
                "exp": expire
            }
            encoded_jwt = jwt.encode(
                to_encode,
                settings.SECRET_KEY,
                algorithm=settings.ALGORITHM
            )
            return encoded_jwt
            
        except Exception as e:
            logger.error(f"Error creating access token: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Could not create access token"
            )

    def get_current_user(self, token: str) -> User:
        """Get current user from JWT token"""
        try:
            # Remove Bearer prefix if present
            if isinstance(token, str) and token.startswith("Bearer "):
                token = token[7:]
                
            payload = jwt.decode(
                token,
                settings.SECRET_KEY,
                algorithms=[settings.ALGORITHM]
            )
            user_id = int(payload.get("sub"))
            
            user = self.db.query(User).filter(User.id == user_id).first()
            if not user:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="User not found"
                )
                
            return user
            
        except jwt.exceptions.ExpiredSignatureError:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Token has expired"
            )
        except (jwt.exceptions.InvalidTokenError, jwt.exceptions.DecodeError):
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not validate credentials"
            )
        except Exception as e:
            logger.error(f"Error validating token: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Could not validate credentials"
            )

    def authenticate_user(self, email: str, password: str) -> Dict[str, Any]:
        """Authenticate user and return token"""
        try:
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
        except SQLAlchemyError as e:
            logger.error(f"Database error authenticating user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Database error",
            )

    def register_user(self, email: str, username: str, password: str) -> User:
        """Register a new user"""
        try:
            # Check if user already exists
            existing_user = self.db.query(User).filter(
                (User.email == email) | (User.username == username)
            ).first()
            
            if existing_user:
                if existing_user.email == email:
                    raise HTTPException(
                        status_code=status.HTTP_400_BAD_REQUEST,
                        detail="Email already registered",
                    )
                else:
                    raise HTTPException(
                        status_code=status.HTTP_400_BAD_REQUEST,
                        detail="Username already taken",
                    )

            # Create new user
            user = User(
                email=email,
                username=username,
                full_name=username,  # Set full_name to username by default
                hashed_password=self.get_password_hash(password),
                is_active=True,
                is_verified=True,
            )
            self.db.add(user)
            self.db.commit()
            self.db.refresh(user)
            logger.info(f"Successfully registered user: {email}")
            return user

        except IntegrityError as e:
            self.db.rollback()
            logger.error(f"Database integrity error: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="User already exists",
            )
        except SQLAlchemyError as e:
            self.db.rollback()
            logger.error(f"Database error registering user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Database error",
            )
        except HTTPException:
            # Re-raise HTTPException directly
            raise
        except Exception as e:
            self.db.rollback()
            logger.error(f"Unexpected error registering user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Internal server error",
            )

    def delete_user(self, user_id: int, current_user: Optional[User] = None) -> User:
        """Delete a user"""
        try:
            # Check if user exists
            user = self.db.query(User).filter(User.id == user_id).first()
            if not user:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail="User not found"
                )
            
            # Check permissions
            if current_user and not current_user.is_superuser:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Insufficient permissions"
                )
            
            # Delete user
            self.db.delete(user)
            self.db.commit()
            
            logger.info(f"Successfully deleted user: {user.email}")
            return user
            
        except HTTPException:
            raise
        except Exception as e:
            self.db.rollback()
            logger.error(f"Error deleting user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Internal server error"
            )
