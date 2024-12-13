from datetime import datetime
from typing import List, Optional
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.features.providers.models.category import ModelCategory
from app.features.providers.schemas.category import CategoryCreate, CategoryUpdate
from app.features.providers.models.model import ProviderModel

class CategoryService:
    def __init__(self, db: AsyncSession):
        self.db = db

    async def create_category(
        self, 
        category_data: CategoryCreate,
        parent_id: Optional[int] = None
    ) -> ModelCategory:
        """Create a new category"""
        now = datetime.utcnow()
        db_category = ModelCategory(
            name=category_data.name,
            description=category_data.description,
            parent_id=parent_id,
            created_at=now,
            updated_at=now
        )
        self.db.add(db_category)
        await self.db.commit()
        await self.db.refresh(db_category)
        return db_category

    async def get_category(self, category_id: int) -> Optional[ModelCategory]:
        """Get a category by ID"""
        query = select(ModelCategory).where(
            ModelCategory.id == category_id,
            ModelCategory.deleted_at.is_(None)
        ).options(selectinload(ModelCategory.models))
        result = await self.db.execute(query)
        return result.scalars().first()

    async def get_category_tree(self, category_id: Optional[int] = None) -> List[ModelCategory]:
        """Get the full category tree"""
        query = select(ModelCategory).where(
            ModelCategory.deleted_at.is_(None)
        )
        if category_id:
            query = query.where(ModelCategory.parent_id == category_id)
        else:
            query = query.where(ModelCategory.parent_id.is_(None))
        
        query = query.options(selectinload(ModelCategory.children))
        result = await self.db.execute(query)
        return list(result.scalars().all())

    async def update_category(
        self,
        category_id: int,
        category_data: CategoryUpdate
    ) -> Optional[ModelCategory]:
        """Update a category"""
        db_category = await self.get_category(category_id)
        if not db_category:
            return None

        update_data = category_data.model_dump(exclude_unset=True)
        for field, value in update_data.items():
            setattr(db_category, field, value)
        
        db_category.updated_at = datetime.utcnow()
        await self.db.commit()
        await self.db.refresh(db_category)
        return db_category

    async def delete_category(self, category_id: int) -> bool:
        """Soft delete a category"""
        db_category = await self.get_category(category_id)
        if not db_category:
            return False

        db_category.deleted_at = datetime.utcnow()
        await self.db.commit()
        return True

    async def add_model_to_category(
        self,
        category_id: int,
        model_id: int
    ) -> bool:
        """Add a model to a category"""
        stmt = select(ProviderModel).where(
            ProviderModel.id == model_id,
            ProviderModel.deleted_at.is_(None)
        )
        result = await self.db.execute(stmt)
        db_model = result.scalars().first()
        if not db_model:
            return False

        db_category = await self.get_category(category_id)
        if not db_category:
            return False

        if db_model not in db_category.models:
            db_category.models.append(db_model)
            await self.db.commit()
        return True

    async def remove_model_from_category(
        self,
        category_id: int,
        model_id: int
    ) -> bool:
        """Remove a model from a category"""
        stmt = select(ProviderModel).where(
            ProviderModel.id == model_id,
            ProviderModel.deleted_at.is_(None)
        )
        result = await self.db.execute(stmt)
        db_model = result.scalars().first()
        if not db_model:
            return False

        db_category = await self.get_category(category_id)
        if not db_category:
            return False

        if db_model in db_category.models:
            db_category.models.remove(db_model)
            await self.db.commit()
        return True
