from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session
from app.db.session import get_db
from app.services.auth import AuthService
from app.schemas.auth import UserCreate, UserLogin, Token

router = APIRouter()

@router.post("/register", response_model=Token)
def register(
    user: UserCreate,
    db: Session = Depends(get_db)
):
    auth_service = AuthService(db)
    user = auth_service.create_user(user)
    access_token = auth_service.create_access_token(user.id)
    return {"access_token": access_token, "token_type": "bearer"}

@router.post("/login", response_model=Token)
def login(
    user_data: UserLogin,
    db: Session = Depends(get_db)
):
    auth_service = AuthService(db)
    user = auth_service.authenticate_user(user_data)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect email or password",
            headers={"WWW-Authenticate": "Bearer"},
        )
    access_token = auth_service.create_access_token(user.id)
    return {"access_token": access_token, "token_type": "bearer"}
