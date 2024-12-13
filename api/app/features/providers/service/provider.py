from datetime import datetime
from typing import List, Optional, Dict, Any
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, or_, update, delete, func, text
from app.features.providers.models.provider import ModelProvider
from app.features.providers.schemas.provider import ProviderCreate, ProviderUpdate
import asyncio

class ProviderService:
    @staticmethod
    async def create_provider(db: AsyncSession, provider: ProviderCreate) -> ModelProvider:
        """Create a new model provider"""
        now = datetime.utcnow()
        db_provider = ModelProvider(
            **provider.model_dump(),
            created_at=now,
            updated_at=now
        )
        db.add(db_provider)
        await db.commit()
        await db.refresh(db_provider)
        return db_provider

    @staticmethod
    async def get_provider(db: AsyncSession, provider_id: int) -> Optional[ModelProvider]:
        """Get a provider by ID"""
        result = await db.execute(
            select(ModelProvider).filter(
                ModelProvider.id == provider_id,
                ModelProvider.deleted_at.is_(None)
            )
        )
        return result.scalar_one_or_none()

    @staticmethod
    async def get_providers(
        db: AsyncSession,
        skip: int = 0,
        limit: int = 100,
        enabled_only: bool = False
    ) -> List[ModelProvider]:
        """Get list of providers with optional filtering"""
        query = select(ModelProvider).filter(ModelProvider.deleted_at.is_(None))
        
        if enabled_only:
            query = query.filter(ModelProvider.enabled == True)
        
        query = query.offset(skip).limit(limit)
        result = await db.execute(query)
        return [provider for provider in result.scalars().all()]

    @staticmethod
    async def update_provider(
        db: AsyncSession,
        provider_id: int,
        update_data: Dict[str, Any]
    ) -> Optional[ModelProvider]:
        """Update provider by id"""
        provider = await ProviderService.get_provider(db, provider_id)
        if not provider:
            return None

        # Convert Pydantic model to dict if necessary
        if hasattr(update_data, "model_dump"):
            update_data = update_data.model_dump(exclude_unset=True)

        # Update provider using SQL UPDATE with Python datetime
        update_values = {
            "updated_at": datetime.utcnow().replace(microsecond=datetime.utcnow().microsecond + 1),
            **{k: v for k, v in update_data.items() if v is not None}
        }
        await db.execute(
            update(ModelProvider)
            .where(ModelProvider.id == provider_id)
            .values(**update_values)
        )
        
        await db.commit()
        await db.refresh(provider)
        return provider

    @staticmethod
    async def delete_provider(db: AsyncSession, provider_id: int) -> bool:
        """Soft delete a provider"""
        db_provider = await ProviderService.get_provider(db, provider_id)
        if not db_provider:
            return False
            
        db_provider.deleted_at = datetime.utcnow()
        await db.commit()
        return True

    @staticmethod
    async def search_providers(
        db: AsyncSession,
        query: str,
        enabled_only: bool = False,
        skip: int = 0,
        limit: int = 100
    ) -> List[ModelProvider]:
        """Search providers by name"""
        db_query = select(ModelProvider).filter(ModelProvider.deleted_at.is_(None))
        
        if query:
            # Ensure case-insensitive search with exact pattern
            db_query = db_query.filter(
                func.lower(ModelProvider.provider_name).like(f"%{query.lower()}%")
            )
        
        if enabled_only:
            db_query = db_query.filter(ModelProvider.enabled == True)
        
        db_query = db_query.offset(skip).limit(limit)
        result = await db.execute(db_query)
        return list(result.scalars().all())
