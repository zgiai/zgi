from datetime import datetime, timedelta
from typing import Optional
import logging
from sqlalchemy.orm import Session
from fastapi import HTTPException, status
from jose import jwt
from passlib.context import CryptContext
from app.core.security import create_access_token
from app.core.config import settings
from app.models import User
from app.features.auth.client.models import APIKey
from app.features.auth.client.schemas import UserCreate, Token

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")

class AuthClientService:
    def __init__(self, db: Session):
        self.db = db

    def create_access_token(self, data: dict) -> str:
        to_encode = data.copy()
        expire = datetime.utcnow() + timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
        to_encode.update({"exp": expire})
        encoded_jwt = jwt.encode(
            to_encode, 
            settings.SECRET_KEY, 
            algorithm=settings.ALGORITHM
        )
        return encoded_jwt

    def verify_password(self, plain_password: str, hashed_password: str) -> bool:
        return pwd_context.verify(plain_password, hashed_password)

    def get_password_hash(self, password: str) -> str:
        return pwd_context.hash(password)

    def get_user_by_email(self, email: str) -> Optional[User]:
        return self.db.query(User).filter(User.email == email).first()

    def get_user_by_id(self, user_id: int) -> Optional[User]:
        return self.db.query(User).filter(User.id == user_id).first()

    def register(self, user_data: UserCreate) -> Token:
        try:
            # Check if user exists
            db_user = self.get_user_by_email(user_data.email)
            if db_user:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Email already registered"
                )

            # Create new user
            hashed_password = self.get_password_hash(user_data.password)
            db_user = User(
                email=user_data.email,
                username=user_data.username,
                hashed_password=hashed_password
            )
            
            self.db.add(db_user)
            self.db.commit()
            self.db.refresh(db_user)

            # Create access token
            access_token = self.create_access_token(
                data={"sub": str(db_user.id)}
            )
            
            logger.info(f"Successfully registered user with email: {user_data.email}")
            return Token(access_token=access_token)
        except Exception as e:
            logger.error(f"Error registering user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def login(self, email: str, password: str) -> Token:
        try:
            # Get user
            user = self.get_user_by_email(email)
            if not user:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Incorrect email or password"
                )

            # Verify password
            if not self.verify_password(password, user.hashed_password):
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Incorrect email or password"
                )

            # Create access token
            access_token = self.create_access_token(
                data={"sub": str(user.id)}
            )
            
            logger.info(f"Successfully logged in user with email: {email}")
            return Token(access_token=access_token)
        except Exception as e:
            logger.error(f"Error logging in user: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def authenticate_api_key(self, api_key: str):
        """Authenticate a client using API key"""
        try:
            api_key_obj = self.db.query(APIKey).filter(APIKey.key == api_key).first()
            if not api_key_obj or not api_key_obj.is_active:
                raise HTTPException(
                    status_code=status.HTTP_401_UNAUTHORIZED,
                    detail="Invalid API key"
                )
            logger.info(f"Successfully authenticated API key: {api_key}")
            return api_key_obj
        except Exception as e:
            logger.error(f"Error authenticating API key: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def create_api_key(self, name: str, key: str):
        """Create a new API key"""
        try:
            api_key = APIKey(name=name, key=key)
            self.db.add(api_key)
            self.db.commit()
            self.db.refresh(api_key)
            logger.info(f"Successfully created API key: {name}")
            return api_key
        except Exception as e:
            logger.error(f"Error creating API key: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def get_api_key(self, api_key_id: int):
        """Get API key by ID"""
        try:
            api_key = self.db.query(APIKey).filter(APIKey.id == api_key_id).first()
            if not api_key:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail="API key not found"
                )
            logger.info(f"Successfully retrieved API key: {api_key_id}")
            return api_key
        except Exception as e:
            logger.error(f"Error retrieving API key: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def delete_api_key(self, api_key_id: int):
        """Delete an API key"""
        try:
            api_key = self.get_api_key(api_key_id)
            self.db.delete(api_key)
            self.db.commit()
            logger.info(f"Successfully deleted API key: {api_key_id}")
            return True
        except Exception as e:
            logger.error(f"Error deleting API key: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def list_api_keys(self, skip: int = 0, limit: int = 100):
        """List all API keys"""
        try:
            return self.db.query(APIKey).offset(skip).limit(limit).all()
        except Exception as e:
            logger.error(f"Error listing API keys: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )

    def create_access_token_for_api_key(self, api_key: APIKey):
        """Create access token for API key"""
        try:
            return {
                "access_token": create_access_token(str(api_key.id)),
                "token_type": "bearer"
            }
        except Exception as e:
            logger.error(f"Error creating access token for API key: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )
