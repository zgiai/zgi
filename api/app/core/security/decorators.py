from functools import wraps
from fastapi import HTTPException, status, Depends
from app.models import User
from app.core.security import get_current_user

def require_admin(func):
    """
    装饰器：检查当前用户是否是管理员
    """
    @wraps(func)
    async def wrapper(*args, current_user: User = Depends(get_current_user), **kwargs):
        if current_user.role != 'admin':
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Admin privileges required"
            )
        return await func(*args, current_user=current_user, **kwargs)
    return wrapper
