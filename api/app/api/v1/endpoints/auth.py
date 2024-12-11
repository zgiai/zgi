from fastapi import APIRouter, Depends, HTTPException, status, Response
from sqlalchemy.orm import Session
from app.db.session import get_db
from app.services.auth import AuthService
from app.schemas.auth import UserCreate, UserLogin, Token, UserResponse
from fastapi.security import OAuth2PasswordBearer

router = APIRouter()
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="/v1/auth/login")

@router.post("/register", response_model=Token, status_code=status.HTTP_201_CREATED)
async def register(
    user: UserCreate,
    db: Session = Depends(get_db)
) -> Token:
    """
    注册新用户并返回访问令牌。
    """
    auth_service = AuthService(db)
    user = auth_service.create_user(user)
    access_token = auth_service.create_access_token(user.id)
    return {"access_token": access_token, "token_type": "bearer"}

@router.post("/login", response_model=Token)
async def login(
    user_data: UserLogin,
    db: Session = Depends(get_db)
) -> Token:
    """
    用户登录并返回访问令牌。
    """
    auth_service = AuthService(db)
    user = auth_service.authenticate_user(user_data)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="邮箱或密码错误",
            headers={"WWW-Authenticate": "Bearer"},
        )
    access_token = auth_service.create_access_token(user.id)
    return {"access_token": access_token, "token_type": "bearer"}

@router.get("/me", response_model=UserResponse)
async def read_users_me(
    token: str = Depends(oauth2_scheme),
    db: Session = Depends(get_db)
) -> UserResponse:
    """
    获取当前用户信息。
    """
    auth_service = AuthService(db)
    current_user = await auth_service.get_current_user(token, db)
    return current_user
