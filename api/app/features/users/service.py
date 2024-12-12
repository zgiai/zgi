from datetime import datetime, timedelta
from typing import Optional, Tuple, List
from sqlalchemy.orm import Session
from fastapi import HTTPException, status
import random

from app.core.security import get_password_hash, verify_password
from app.core.security.auth import create_access_token
from app.features.users.models import User
from app.features.users.schemas import (
    UserProfileUpdate,
    UserPreferences,
    UserLogin,
    UserCreate,
    Token
)
from app.core.email import EmailService
from app.core.cache import Cache

class UserService:
    def __init__(self, db: Session, cache: Cache, email_service: EmailService):
        self.db = db
        self.cache = cache
        self.email_service = email_service

    async def authenticate(self, user_data: UserLogin) -> Optional[Token]:
        """用户登录认证"""
        user = self.db.query(User).filter(User.email == user_data.email).first()
        if not user or not verify_password(user_data.password, user.hashed_password):
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="邮箱或密码错误",
                headers={"WWW-Authenticate": "Bearer"},
            )
        
        if not user.is_active:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="用户已被停用"
            )

        # 生成访问令牌
        access_token = create_access_token(data={"sub": user.email})
        return Token(access_token=access_token, token_type="bearer")

    async def create_user(self, user_data: UserCreate) -> User:
        """创建新用户"""
        # 检查邮箱是否已存在
        if self.db.query(User).filter(User.email == user_data.email).first():
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="该邮箱已被注册"
            )
            
        # 检查用户名是否已存在
        if self.db.query(User).filter(User.username == user_data.username).first():
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="该用户名已被使用"
            )

        # 创建新用户
        user = User(
            email=user_data.email,
            username=user_data.username,
            full_name=user_data.full_name,
            hashed_password=get_password_hash(user_data.password),
            is_active=True,
            is_verified=False,
            is_admin=False,
            is_superuser=False,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow()
        )
        self.db.add(user)
        self.db.commit()
        self.db.refresh(user)
        return user

    async def get_user_profile(self, user_id: int) -> Optional[User]:
        """获取用户信息"""
        # 尝试从缓存获取
        cache_key = f"user_profile:{user_id}"
        if user := await self.cache.get(cache_key):
            return user

        user = self.db.query(User).filter(User.id == user_id).first()
        if user:
            # 更新登录次数和最后登录时间
            if not user.last_login:
                user.login_count = 1
            else:
                user.login_count += 1
            user.last_login = datetime.utcnow()
            self.db.commit()
            
            # 使用较短的缓存时间以保持数据新鲜度
            await self.cache.set(cache_key, user, expire=1800)  # 缓存30分钟
        return user

    async def update_profile(self, user_id: int, update_data: UserProfileUpdate) -> User:
        """更新用户信息"""
        user = await self.get_user_profile(user_id)
        if not user:
            raise HTTPException(status_code=404, detail="用户不存在")

        update_dict = update_data.dict(exclude_unset=True)
        
        # 处理密码更新
        if "password" in update_dict:
            # 密码强度验证已在schema中完成
            update_dict["hashed_password"] = get_password_hash(update_dict.pop("password"))

        # 验证网站URL格式（如果提供）
        if website := update_dict.get("website"):
            if not website.startswith(("http://", "https://")):
                update_dict["website"] = f"https://{website}"

        # 更新用户信息
        for field, value in update_dict.items():
            setattr(user, field, value)

        user.updated_at = datetime.utcnow()
        self.db.commit()
        self.db.refresh(user)
        
        # 清除缓存
        await self.cache.delete(f"user_profile:{user_id}")
        return user

    async def update_preferences(self, user_id: int, preferences: UserPreferences) -> User:
        """更新用户偏好设置"""
        user = await self.get_user_profile(user_id)
        if not user:
            raise HTTPException(status_code=404, detail="用户不存在")

        # 转换偏好设置为字典并验证
        prefs_dict = preferences.dict(exclude_unset=True)
        
        # 验证时区
        if timezone := prefs_dict.get("timezone"):
            try:
                import pytz
                pytz.timezone(timezone)
            except pytz.exceptions.UnknownTimeZoneError:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail=f"无效的时区: {timezone}"
                )

        # 合并现有偏好设置
        current_prefs = user.preferences or {}
        current_prefs.update(prefs_dict)
        user.preferences = current_prefs
        
        user.updated_at = datetime.utcnow()
        self.db.commit()
        self.db.refresh(user)
        
        # 清除缓存
        await self.cache.delete(f"user_profile:{user_id}")
        return user

    async def request_password_reset(self, email: str) -> bool:
        """请求密码重置，发送验证码到用户邮箱"""
        user = self.db.query(User).filter(User.email == email).first()
        if not user:
            # 即使用户不存在也返回成功，避免泄露用户信息
            return True

        # 生成6位数字验证码
        reset_code = ''.join(random.choices('0123456789', k=6))
        
        # 存储验证码到缓存，设置15分钟过期
        cache_key = f"password_reset:{email}"
        await self.cache.set(cache_key, reset_code, expire=900)

        # 发送重置邮件
        await self.email_service.send_email(
            to_email=email,
            subject="密码重置验证码",
            content=f"""
            您好，

            您正在重置密码。您的验证码是：{reset_code}

            该验证码将在15分钟后过期。如果这不是您本人的操作，请忽略此邮件。

            祝好，
            ZGI团队
            """
        )
        return True

    async def verify_reset_code(self, email: str, reset_code: str) -> bool:
        """验证重置码是否正确"""
        cache_key = f"password_reset:{email}"
        stored_code = await self.cache.get(cache_key)
        
        if not stored_code or stored_code != reset_code:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="验证码无效或已过期"
            )
        return True

    async def reset_password(self, email: str, reset_code: str, new_password: str) -> bool:
        """重置密码"""
        # 验证重置码
        await self.verify_reset_code(email, reset_code)

        # 获取用户
        user = self.db.query(User).filter(User.email == email).first()
        if not user:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="用户不存在"
            )

        # 更新密码
        user.hashed_password = get_password_hash(new_password)
        user.updated_at = datetime.utcnow()
        self.db.commit()

        # 清除重置码缓存
        cache_key = f"password_reset:{email}"
        await self.cache.delete(cache_key)

        # 发送密码修改通知邮件
        await self.email_service.send_email(
            to_email=email,
            subject="密码已重置",
            content="""
            您好，

            您的密码已经成功重置。如果这不是您本人的操作，请立即联系我们。

            祝好，
            ZGI团队
            """
        )
        return True

    async def send_verification_email(self, user_id: int) -> bool:
        """发送验证邮件"""
        user = await self.get_user_profile(user_id)
        if not user or user.is_verified:
            return False

        # 生成验证令牌
        verification_token = create_access_token(
            data={"sub": user.email, "type": "email_verification"},
            expires_delta=timedelta(hours=24)
        )

        # 存储令牌到缓存
        await self.cache.set(
            f"email_verification:{user.email}",
            verification_token,
            expire=86400  # 24小时
        )

        # 发送验证邮件
        await self.email_service.send_verification_email(
            email=user.email,
            token=verification_token
        )
        return True

    async def verify_email(self, email: str, token: str) -> bool:
        """验证邮箱"""
        # 验证令牌
        cached_token = await self.cache.get(f"email_verification:{email}")
        if not cached_token or cached_token != token:
            return False

        user = self.db.query(User).filter(User.email == email).first()
        if not user:
            return False

        user.is_verified = True
        user.updated_at = datetime.utcnow()
        self.db.commit()

        # 清除验证令牌
        await self.cache.delete(f"email_verification:{email}")
        return True

    async def list_users(
        self, skip: int = 0, limit: int = 10
    ) -> Tuple[List[User], int]:
        """获取用户列表"""
        total = self.db.query(User).count()
        users = self.db.query(User).offset(skip).limit(limit).all()
        return users, total

    async def deactivate_user(self, user_id: int) -> bool:
        """停用用户"""
        user = await self.get_user_profile(user_id)
        if not user:
            return False

        user.is_active = False
        user.updated_at = datetime.utcnow()
        self.db.commit()
        
        # 清除缓存
        await self.cache.delete(f"user_profile:{user_id}")
        return True
