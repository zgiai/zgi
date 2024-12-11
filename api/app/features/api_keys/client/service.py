from datetime import datetime, timedelta
from typing import List, Optional
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select
from sqlalchemy import update
from fastapi import HTTPException, status
from sqlalchemy.exc import IntegrityError

from app.core.config import settings
from app.models import APIKey
from app.features.api_keys.client.schemas import APIKeyCreate, APIKeyUpdate

class APIKeyClientService:
    def __init__(self, db: AsyncSession):
        self.db = db

    async def create_api_key(self, user_id: int, api_key_data: APIKeyCreate) -> APIKey:
        db_api_key = APIKey(
            user_id=user_id,
            **api_key_data.dict()
        )
        self.db.add(db_api_key)
        await self.db.commit()
        await self.db.refresh(db_api_key)
        return db_api_key

    async def get_user_api_keys(self, user_id: int) -> List[APIKey]:
        result = await self.db.execute(
            select(APIKey)
            .where(APIKey.user_id == user_id)
            .order_by(APIKey.created_at.desc())
        )
        return result.scalars().all()

    async def get_api_key(self, user_id: int, api_key_id: int) -> Optional[APIKey]:
        result = await self.db.execute(
            select(APIKey)
            .where(APIKey.id == api_key_id, APIKey.user_id == user_id)
        )
        return result.scalar_one_or_none()

    async def update_api_key(
        self, 
        user_id: int, 
        api_key_id: int, 
        api_key_data: APIKeyUpdate
    ) -> Optional[APIKey]:
        api_key = await self.get_api_key(user_id, api_key_id)
        if not api_key:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="API key not found"
            )

        update_data = api_key_data.dict(exclude_unset=True)
        for key, value in update_data.items():
            setattr(api_key, key, value)

        await self.db.commit()
        await self.db.refresh(api_key)
        return api_key

    async def delete_api_key(self, user_id: int, api_key_id: int) -> bool:
        api_key = await self.get_api_key(user_id, api_key_id)
        if not api_key:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="API key not found"
            )

        await self.db.delete(api_key)
        await self.db.commit()
        return True

    async def update_last_used(self, api_key_id: int) -> None:
        await self.db.execute(
            update(APIKey)
            .where(APIKey.id == api_key_id)
            .values(last_used_at=datetime.utcnow())
        )
        await self.db.commit()
