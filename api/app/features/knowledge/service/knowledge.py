from typing import List, Optional, Tuple, Dict, Any
from uuid import uuid4

from sqlalchemy.orm import Session, joinedload, selectinload
from sqlalchemy import and_, or_, select, func
from sqlalchemy.ext.asyncio import AsyncSession
from fastapi import HTTPException

from app.features.knowledge.models.knowledge import KnowledgeBase, Visibility, Status
from app.features.knowledge.models.document import Document, DocumentStatus
from app.features.knowledge.schemas.request.knowledge import (
    KnowledgeBaseCreate,
    KnowledgeBaseUpdate,
    SearchQuery
)
from app.features.knowledge.schemas.response.knowledge import KnowledgeBaseList
from app.features.knowledge.vector.factory import VectorDBFactory
from app.features.knowledge.embedding.factory import EmbeddingFactory
from app.features.knowledge.core.config import (
    get_vector_db_settings,
    get_embedding_settings,
    get_knowledge_base_settings
)
from app.features.knowledge.core.response import (
    ServiceResponse,
    NotFoundError,
    ValidationError,
    AuthorizationError,
    ServiceError
)
from app.features.knowledge.core.decorators import (
    handle_service_errors,
    retry,
    async_cache
)
from app.features.knowledge.core.logging import (
    service_logger,
    audit_logger,
    metrics_logger
)


class KnowledgeBaseService:
    """Service for managing knowledge bases"""

    def __init__(self, db: Session):
        self.db = db
        self.kb_settings = get_knowledge_base_settings()

        # Initialize vector DB
        vector_settings = get_vector_db_settings()
        self.vector_db = VectorDBFactory.create(
            vector_settings.PROVIDER,
            **vector_settings.provider_config
        )

        # Initialize embedding service
        self.embedding_settings = get_embedding_settings()
        self.embedding = EmbeddingFactory.create(
            self.embedding_settings.PROVIDER,
            **self.embedding_settings.provider_config
        )

    @handle_service_errors
    async def create_knowledge_base(
            self,
            kb_create: KnowledgeBaseCreate,
            user_id: int
    ) -> ServiceResponse[KnowledgeBase]:
        """Create a new knowledge base"""
        try:
            # Validate input
            if kb_create.visibility == Visibility.ORGANIZATION and not kb_create.organization_id:
                raise ValidationError("Organization ID is required for organization visibility")

            # check collection_name
            # query = select(KnowledgeBase).where(KnowledgeBase.name == kb_create.name)
            # result = await self.db.execute(query)
            # exist_kb = result.scalars().first()
            # exist_kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.name == kb_create.name).first()
            # if exist_kb:
            #     raise ValidationError("Knowledge base with this name already exists")
            collection_name = f"kb_{user_id}_{str(uuid4())[:8]}"
            # Create knowledge base
            # get model from embedding setting
            kb_create.model = self.embedding_settings.MODEL
            embedding_dimension = self.embedding.get_dimension()
            kb = KnowledgeBase(
                name=kb_create.name,
                description=kb_create.description,
                visibility=kb_create.visibility,
                owner_id=user_id,
                organization_id=kb_create.organization_id,
                model=kb_create.model or self.kb_settings.DEFAULT_EMBEDDING_MODEL,
                dimension=embedding_dimension,
                collection_name=collection_name,
                meta_info=kb_create.metadata,
                tags=kb_create.tags,
                status=Status.ACTIVE.value  # Use the value instead of the enum
            )

            # Create vector collection
            success = await self.vector_db.create_collection(
                kb.collection_name,
                dimension=kb.dimension
            )
            if not success:
                raise HTTPException(status_code=500, detail="Failed to create vector collection")

            # Save to database
            self.db.add(kb)
            # await self.db.commit()
            # await self.db.refresh(kb)
            self.db.commit()
            self.db.refresh(kb)

            # Log audit
            audit_logger.log_access(
                user_id,
                "knowledge_base",
                str(kb.id),
                "create",
                "success"
            )

            # Return response
            return ServiceResponse(
                success=True,
                code=200,
                message="Created",
                data=kb.to_dict()
            )

        except Exception as e:
            self.db.rollback()
            # Try to cleanup vector collection if created
            try:
                self.vector_db.delete_collection(kb.collection_name)
            except:
                pass
            raise

    @handle_service_errors
    @async_cache(ttl=3600)
    async def get_knowledge_base(
            self,
            kb_id: int,
            user_id: int
    ) -> ServiceResponse[KnowledgeBase]:
        """Get a knowledge base by ID"""
        # query = select(KnowledgeBase).where(KnowledgeBase.id == kb_id)
        # result = await self.db.execute(query)
        # kb = result.scalars().first()
        kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.id == kb_id).first()
        if not kb:
            raise NotFoundError("Knowledge base not found")

        # Check access
        if not self._can_access(kb, user_id):
            raise AuthorizationError("Not authorized to access this knowledge base")

        audit_logger.log_access(
            user_id,
            "knowledge_base",
            str(kb_id),
            "read",
            "success"
        )

        return ServiceResponse(
            success=True,
            code=200,
            message="Get",
            data=kb.to_dict()
        )

    @handle_service_errors
    async def update_knowledge_base(
            self,
            kb_id: int,
            kb_update: KnowledgeBaseUpdate,
            user_id: int
    ) -> ServiceResponse[KnowledgeBase]:
        """Update a knowledge base"""
        # query = select(KnowledgeBase).where(KnowledgeBase.id == kb_id)
        # result = await self.db.execute(query)
        # kb = result.scalars().first()
        kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.id == kb_id).first()
        if not kb:
            raise NotFoundError(f"Knowledge base {kb_id} not found")

        # Check ownership
        if kb.owner_id != user_id:
            raise AuthorizationError("Not authorized to update this knowledge base")

        # Update fields
        for field, value in kb_update.dict(exclude_unset=True).items():
            setattr(kb, field, value)
        self.db.add(kb)
        # await self.db.commit()
        # await self.db.refresh(kb)
        self.db.commit()
        self.db.refresh(kb)

        audit_logger.log_access(
            user_id,
            "knowledge_base",
            str(kb_id),
            "update",
            "success"
        )

        return ServiceResponse(
            success=True,
            code=200,
            message="update",
            data=kb.to_dict()
        )

    async def delete_knowledge_base(
            self,
            kb_id: int,
            user_id: int
    ) -> KnowledgeBase:
        """Delete a knowledge base"""
        # query = select(KnowledgeBase).where(KnowledgeBase.id == kb_id)
        # result = await self.db.execute(query)
        # kb = result.scalars().first()
        kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.id == kb_id).first()
        if not kb:
            raise NotFoundError(f"Knowledge base {kb_id} not found")

        # Check ownership
        if kb.owner_id != user_id:
            raise AuthorizationError("Not authorized to delete this knowledge base")

        try:
            # Delete vector collection
            await self.vector_db.delete_collection(kb.collection_name)

            # Mark as deleted in database
            kb.status = Status.DELETED.value
            self.db.add(kb)
            # await self.db.commit()
            self.db.commit()

            audit_logger.log_access(
                user_id,
                "knowledge_base",
                str(kb_id),
                "delete",
                "success"
            )

            return kb

        except Exception as e:
            self.db.rollback()
            raise

    def _can_access(self, kb: KnowledgeBase, user_id: int) -> bool:
        """Check if user can access knowledge base"""
        if kb.status == Status.DELETED.value:
            return False

        if kb.owner_id == user_id:
            return True

        if kb.visibility == Visibility.PUBLIC:
            return True

        if kb.visibility == Visibility.ORGANIZATION:
            # TODO: Check organization membership
            return True

        return False

    @handle_service_errors
    async def list_knowledge_bases(
            self,
            user_id: int,
            page_num: int = 1,
            page_size: int = 10,
            organization_id: Optional[int] = None,
            query_name: Optional[str] = None
    ) -> ServiceResponse[KnowledgeBaseList]:
        """List knowledge bases"""
        try:
            # Build query
            # 计算偏移量
            skip = (page_num - 1) * page_size

            # 构建查询
            # query = select(KnowledgeBase).where(
            #     and_(
            #         KnowledgeBase.status != Status.DELETED.value,
            #         or_(
            #             KnowledgeBase.owner_id == user_id,
            #             KnowledgeBase.visibility == Visibility.PUBLIC,
            #             and_(
            #                 KnowledgeBase.visibility == Visibility.ORGANIZATION,
            #                 KnowledgeBase.organization_id == organization_id
            #             ) if organization_id else False
            #         )
            #     )
            # )
            query_filter = and_(
                KnowledgeBase.status != Status.DELETED.value,
                or_(
                    KnowledgeBase.owner_id == user_id, KnowledgeBase.visibility == Visibility.PUBLIC,
                    and_(
                        KnowledgeBase.visibility == Visibility.ORGANIZATION,
                        KnowledgeBase.organization_id == organization_id
                    ) if organization_id else False
                )
            )

            if query_name:
                query_filter = and_(
                    query_filter,
                    KnowledgeBase.name.ilike(f"%{query_name}%")
                )

            query = self.db.query(KnowledgeBase).filter(
                query_filter
            )

            # Print query for debugging
            print(f"Query: {query}")
            print(f"User ID: {user_id}")

            # Get total count
            # count_query = select(func.count()).select_from(query.subquery())
            # total = await self.db.scalar(count_query) or 0
            total = query.count() or 0

            print(f"Total: {total}")
            # Get items with pagination
            query = (query.order_by(KnowledgeBase.created_at.desc())
                     .offset(skip).limit(page_size))
            # result = await self.db.execute(query)
            # items = result.scalars().all()
            items = query.all()

            print(f"Items: {items}")

            # Convert to response model
            response_items = [
                {**kb.to_dict(), "owner_name": kb.owner.username if kb.owner else None}
                for kb in items]
            response = {"total": total, "items": response_items}

            return ServiceResponse(
                success=True,
                code=200,
                message="Success",
                data=response
            )

        except Exception as e:
            print(f"Error: {str(e)}")
            raise ServiceError(str(e))

    @handle_service_errors(wrap_response=False)
    async def search(
            self,
            kb_id: int,
            query: SearchQuery,
            user_id: int
    ) -> List[Dict[str, Any]]:
        """Search documents in a knowledge base"""
        # Get knowledge base and check access
        # query_knowledge = select(KnowledgeBase).where(KnowledgeBase.id == kb_id)
        # query_result = await self.db.execute(query_knowledge)
        # kb = query_result.scalars().first()
        kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.id == kb_id).first()

        if not kb:
            raise NotFoundError(f"Knowledge base {kb_id} not found")

        if not self._can_access(kb, user_id):
            raise AuthorizationError("Not authorized to access this knowledge base")

        try:
            # Generate query embedding
            embeddings = await self.embedding.get_embeddings([query.text])
            if not embeddings:
                raise Exception("Failed to generate query embedding")

            # Search vector database
            results = await self.vector_db.search(
                collection_name=kb.collection_name,
                query_vector=embeddings[0],
                top_k=query.top_k,
                filters=None
            )

            # Log access
            audit_logger.log_access(
                user_id,
                "knowledge_base",
                str(kb_id),
                "read",
                "success"
            )

            return results

        except Exception as e:
            raise

    async def clone_knowledge_base(
            self,
            kb_id: int,
            name: str,
            user_id: int
    ):
        """Clone a knowledge base"""
        original_kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.id == kb_id).first()

        if not original_kb:
            raise NotFoundError(f"Knowledge base {kb_id} not found")

        if not self._can_access(original_kb, user_id):
            raise AuthorizationError("Not authorized to access this knowledge base")

        if not name:
            name = f"{original_kb.name} (Clone)"
        if name == original_kb.name:
            raise ServiceError("New name cannot be the same as the original name")
        collection_name = f"kb_{user_id}_{str(uuid4())[:8]}"
        new_kb = KnowledgeBase(
            name=name,
            description=original_kb.description,
            visibility=original_kb.visibility,
            owner_id=user_id,
            organization_id=original_kb.organization_id,
            model=original_kb.model,
            dimension=original_kb.dimension,
            collection_name=collection_name,
            meta_info=original_kb.meta_info,
            tags=original_kb.tags,
            status=Status.ACTIVE.value
        )

        success = await self.vector_db.create_collection(
            new_kb.collection_name,
            dimension=new_kb.dimension
        )
        if not success:
            raise HTTPException(status_code=500, detail="Failed to create vector collection")

        # 保存到数据库
        self.db.add(new_kb)
        # await self.db.commit()
        # await self.db.refresh(new_kb)
        self.db.commit()
        self.db.refresh(new_kb)

        # 记录审计日志
        audit_logger.log_access(
            user_id,
            "knowledge_base",
            str(new_kb.id),
            "clone",
            "success"
        )

        return new_kb, original_kb
