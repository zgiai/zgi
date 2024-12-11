from fastapi import APIRouter, Depends, HTTPException, status, Header
from fastapi.security import OAuth2PasswordBearer
from jose import JWTError, jwt
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.config import settings
from app.models import User
from app.features.auth.client.schemas import UserCreate, UserLogin, Token, UserResponse
from app.features.auth.client.service import AuthClientService

router = APIRouter(prefix="/v1/client/auth", tags=["client-auth"])

oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/client/auth/login")

def get_current_user(
    token: str = Depends(oauth2_scheme),
    db: Session = Depends(get_db)
) -> User:
    credentials_exception = HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Could not validate credentials",
        headers={"WWW-Authenticate": "Bearer"},
    )
    try:
        payload = jwt.decode(token, settings.SECRET_KEY, algorithms=[settings.ALGORITHM])
        user_id: str = payload.get("sub")
        if user_id is None:
            raise credentials_exception
    except JWTError:
        raise credentials_exception

    auth_service = AuthClientService(db)
    user = auth_service.get_user_by_id(int(user_id))
    if user is None:
        raise credentials_exception
    return user

@router.post("/register", response_model=Token)
def register(
    user_data: UserCreate,
    db: Session = Depends(get_db)
):
    auth_service = AuthClientService(db)
    return auth_service.register(user_data)

@router.post("/login")
def login(
    api_key: str = Header(..., alias="X-API-Key"),
    db: Session = Depends(get_db)
):
    """Client login endpoint using API key"""
    auth_service = AuthClientService(db)
    api_key_obj = auth_service.authenticate_api_key(api_key)
    return auth_service.create_access_token_for_api_key(api_key_obj)

@router.get("/me", response_model=UserResponse)
def get_user_me(
    current_user: User = Depends(get_current_user)
):
    return current_user

# Protected resource endpoint for testing
@router.get("/protected-resource")
def protected_resource(current_user: User = Depends(get_current_user)):
    """Protected resource endpoint for testing"""
    return {
        "message": "You have access to this protected resource",
        "api_key_id": current_user.id
    }
