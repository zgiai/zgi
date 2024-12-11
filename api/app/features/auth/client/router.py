from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer
from jose import JWTError, jwt
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.config import settings
from app.models import User
from app.features.auth.client.schemas import UserCreate, UserLogin, Token, UserResponse
from app.features.auth.client.service import AuthClientService

router = APIRouter()

oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/auth/login")

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

@router.post("/login", response_model=Token)
def login(
    user_data: UserLogin,
    db: Session = Depends(get_db)
):
    auth_service = AuthClientService(db)
    return auth_service.login(user_data.email, user_data.password)

@router.get("/me", response_model=UserResponse)
def get_user_me(
    current_user: User = Depends(get_current_user)
):
    return current_user
