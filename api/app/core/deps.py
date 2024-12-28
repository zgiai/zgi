from typing import Generator, Optional
from fastapi import Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer
from jose import jwt, JWTError
from sqlalchemy.orm import Session
from app.core.database import SessionLocal, get_sync_db
from app.core.config import settings
from app.features.users.models import User

oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/console/auth/login")

# def get_db() -> Generator:
#     """
#     获取数据库会话
#     """
#     try:
#         db = SessionLocal()
#         yield db
#     finally:
#         db.close()

async def get_current_user(
    db: Session = Depends(get_sync_db),
    token: str = Depends(oauth2_scheme)
) -> Optional[User]:
    """
    获取当前用户
    """
    credentials_exception = HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Could not validate credentials",
        headers={"WWW-Authenticate": "Bearer"},
    )
    try:
        payload = jwt.decode(
            token, settings.SECRET_KEY, algorithms=[settings.ALGORITHM]
        )
        user_id: str = payload.get("sub")
        if user_id is None:
            raise credentials_exception
    except JWTError:
        raise credentials_exception

    user = db.query(User).filter(User.id == user_id).first()
    if user is None:
        raise credentials_exception
    return user
