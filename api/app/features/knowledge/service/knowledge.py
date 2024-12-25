from typing import List, Optional, Tuple, Dict, Any
from sqlalchemy.orm import Session
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

    def __init__(self, db: AsyncSession):
        self.db = db
        self.kb_settings = get_knowledge_base_settings()
        
        # Initialize vector DB
        vector_settings = get_vector_db_settings()
        self.vector_db = VectorDBFactory.create(
            vector_settings.PROVIDER,
            **vector_settings.provider_config
        )
        
        # Initialize embedding service
        embedding_settings = get_embedding_settings()
        self.embedding = EmbeddingFactory.create(
            embedding_settings.PROVIDER,
            **embedding_settings.provider_config
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

            # Create knowledge base
            kb = KnowledgeBase(
                name=kb_create.name,
                description=kb_create.description,
                visibility=kb_create.visibility,
                owner_id=user_id,
                organization_id=kb_create.organization_id,
                model=kb_create.model or self.kb_settings.DEFAULT_EMBEDDING_MODEL,
                dimension=self.kb_settings.DEFAULT_EMBEDDING_DIMENSION,
                collection_name=f"kb_{user_id}_{kb_create.name.lower().replace(' ', '_')}",
                metadata=kb_create.metadata,
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
            await self.db.commit()
            await self.db.refresh(kb)
            
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
                code=201,
                message="Created",
                data=kb.to_dict()
            )
            
        except Exception as e:
            await self.db.rollback()
            # Try to cleanup vector collection if created
            try:
                await self.vector_db.delete_collection(kb.collection_name)
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
        kb = await self.db.query(KnowledgeBase).filter(
            KnowledgeBase.id == kb_id
        ).first()
        
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
        
        return ServiceResponse.ok(kb)

    @handle_service_errors
    async def update_knowledge_base(
        self,
        kb_id: int,
        kb_update: KnowledgeBaseUpdate,
        user_id: int
    ) -> ServiceResponse[KnowledgeBase]:
        """Update a knowledge base"""
        kb = await self.db.query(KnowledgeBase).filter(
            KnowledgeBase.id == kb_id
        ).first()
        
        if not kb:
            raise NotFoundError(f"Knowledge base {kb_id} not found")
            
        # Check ownership
        if kb.owner_id != user_id:
            raise AuthorizationError("Not authorized to update this knowledge base")
            
        # Update fields
        for field, value in kb_update.dict(exclude_unset=True).items():
            setattr(kb, field, value)
            
        await self.db.commit()
        await self.db.refresh(kb)
        
        audit_logger.log_access(
            user_id,
            "knowledge_base",
            str(kb_id),
            "update",
            "success"
        )
        
        return ServiceResponse.ok(kb)

    @handle_service_errors
    async def delete_knowledge_base(
        self,
        kb_id: int,
        user_id: int
    ) -> ServiceResponse[None]:
        """Delete a knowledge base"""
        kb = await self.db.query(KnowledgeBase).filter(
            KnowledgeBase.id == kb_id
        ).first()
        
        if not kb:
            raise NotFoundError(f"Knowledge base {kb_id} not found")
            
        # Check ownership
        if kb.owner_id != user_id:
            raise AuthorizationError("Not authorized to delete this knowledge base")
            
        try:
            # Delete vector collection
            await self.vector_db.delete_collection(kb.collection_name)
            
            # Mark as deleted in database
            kb.status = Status.DELETED
            await self.db.commit()
            
            audit_logger.log_access(
                user_id,
                "knowledge_base",
                str(kb_id),
                "delete",
                "success"
            )
            
            return ServiceResponse.ok()
            
        except Exception as e:
            await self.db.rollback()
            raise

    def _can_access(self, kb: KnowledgeBase, user_id: int) -> bool:
        """Check if user can access knowledge base"""
        if kb.status == Status.DELETED:
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
        skip: int = 0,
        limit: int = 10,
        organization_id: Optional[int] = None
    ) -> ServiceResponse[KnowledgeBaseList]:
        """List knowledge bases"""
        try:
            # Build query
            query = select(KnowledgeBase).where(
                and_(
                    KnowledgeBase.status != Status.DELETED.value,
                    or_(
                        KnowledgeBase.owner_id == user_id,
                        KnowledgeBase.visibility == Visibility.PUBLIC,
                        and_(
                            KnowledgeBase.visibility == Visibility.ORGANIZATION,
                            KnowledgeBase.organization_id == organization_id
                        ) if organization_id else False
                    )
                )
            )
            
            # Print query for debugging
            print(f"Query: {query}")
            print(f"User ID: {user_id}")
            
            # Get total count
            count_query = select(func.count()).select_from(query.subquery())
            total = await self.db.scalar(count_query) or 0
            
            print(f"Total: {total}")
            
            # Get items with pagination
            query = query.offset(skip).limit(limit)
            result = await self.db.execute(query)
            items = result.scalars().all()
            
            print(f"Items: {items}")
            
            # Convert to response model
            response_items = [kb.to_dict() for kb in items]
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
        kb = await self.db.query(KnowledgeBase).filter(
            KnowledgeBase.id == kb_id
        ).first()
        
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
                collection_name=f"kb_{kb_id}",
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
