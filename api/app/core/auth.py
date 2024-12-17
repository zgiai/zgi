from datetime import datetime, timedelta
from typing import Optional
from fastapi import Depends, HTTPException, status, Security
from fastapi.security import OAuth2PasswordBearer, HTTPAuthorizationCredentials, HTTPBearer
from jose import JWTError, jwt
from sqlalchemy.orm import Session

from app.core.config import settings
from app.core.database import get_db, get_sync_db
from app.features.organizations.models import OrganizationMember
from app.features.users.models import User
from app.features.auth.service import AuthService, get_auth_service

# JWT相关配置
SECRET_KEY = settings.SECRET_KEY
ALGORITHM = settings.ALGORITHM
ACCESS_TOKEN_EXPIRE_MINUTES = settings.ACCESS_TOKEN_EXPIRE_MINUTES

oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/console/auth/login")

security = HTTPBearer(auto_error=False)

def create_access_token(data: dict, expires_delta: Optional[timedelta] = None):
    to_encode = data.copy()
    if expires_delta:
        expire = datetime.utcnow() + expires_delta
    else:
        expire = datetime.utcnow() + timedelta(minutes=15)
    to_encode.update({"exp": expire})
    encoded_jwt = jwt.encode(to_encode, SECRET_KEY, algorithm=ALGORITHM)
    return encoded_jwt

async def get_current_user(
    db: Session = Depends(get_sync_db),
    token: str = Depends(oauth2_scheme)
) -> User:
    credentials_exception = HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Could not validate credentials",
        headers={"WWW-Authenticate": "Bearer"},
    )
    try:
        payload = jwt.decode(token, SECRET_KEY, algorithms=[ALGORITHM])
        user_id: str = payload.get("sub")
        if user_id is None:
            raise credentials_exception
    except JWTError:
        raise credentials_exception
    
    user = db.query(User).filter(User.id == int(user_id)).first()
    if user is None:
        raise credentials_exception
    if not user.is_active and not user.is_superuser:
        raise HTTPException(status_code=400, detail="Inactive user")
    return user

async def get_api_key(credentials: HTTPAuthorizationCredentials = Security(security)) -> str:
    """Get API key from Authorization header.
    
    Args:
        credentials: Authorization credentials from request header
    
    Returns:
        The API key string
        
    Raises:
        HTTPException: If no valid API key is found
    """
    if not credentials:
        raise HTTPException(status_code=401, detail="No API key provided")
    if not credentials.scheme == "Bearer":
        raise HTTPException(status_code=401, detail="Invalid authentication scheme")
    return credentials.credentials

def require_super_admin(
    token: str = Depends(oauth2_scheme),
    db: Session = Depends(get_sync_db)
) -> User:
    """Require super admin access"""
    auth_service = AuthService(db)
    user = auth_service.get_current_user(token)
    if not user.is_superuser:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Insufficient permissions. Super admin access required."
        )
    return user
