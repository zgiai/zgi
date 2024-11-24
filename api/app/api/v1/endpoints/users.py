from fastapi import APIRouter, Depends
from app.services.users_service import get_user_info
from app.models.users import get_current_user_id
from app.api.base import APIBase

router = APIRouter()

@router.get("/")
@APIBase.api_response(msg="success")
async def read_users():
    return{"test":'ok'}

@router.get("/me")
@APIBase.api_response()
async def read_users_me(current_user_id: int = Depends(get_current_user_id)):
    try:
        user_info = await get_user_info(current_user_id)
        if not user_info:
            return APIBase.error(msg="用户不存在")
        return user_info
        
    except Exception as e:
        return APIBase.error(msg="获取用户信息失败")

