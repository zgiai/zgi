from typing import Optional, Dict, Any, List
from fastapi import APIRouter, Depends, File, UploadFile, Query, HTTPException, status
from sqlalchemy.orm import Session

from app.core.base import resp_200
from app.core.database import get_db
from app.core.deps import get_current_user
from app.features.knowledge.models.knowledge import Visibility, Status
from app.features.knowledge.schemas.request.knowledge import (
    KnowledgeBaseCreate,
    KnowledgeBaseUpdate,
    SearchQuery
)
from app.features.knowledge.schemas.request.document import (
    DocumentUpload,
    DocumentUpdate
)
from app.features.knowledge.schemas.response.knowledge import (
    KnowledgeBaseResponse,
    KnowledgeBaseList,
    SearchResponse
)
from app.features.knowledge.schemas.response.document import (
    DocumentResponse,
    DocumentList,
    DocumentChunkResponse,
    DocumentChunkList
)
from app.features.knowledge.service.knowledge import KnowledgeBaseService
from app.features.knowledge.service.document import DocumentService

# Create router with prefix
router = APIRouter(tags=["Knowledge Base"])

# Knowledge Base Routes

@router.get("/stats",
    summary="Get system-wide statistics")
async def get_stats(
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Get system-wide knowledge base statistics"""
    service = KnowledgeBaseService(db)
    return resp_200("get stats")

@router.post("/create",
    # response_model=KnowledgeBaseResponse,
    summary="Create knowledge base")
async def create_knowledge_base(
    kb_create: KnowledgeBaseCreate,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Create a new knowledge base"""
    service = KnowledgeBaseService(db)
    result = await service.create_knowledge_base(kb_create, current_user.id)
    return resp_200(result.data)

@router.get("/",
    summary="List knowledge bases")
async def list_knowledge_bases(
    page_num: int = Query(1, ge=1),
    page_size: int = Query(10, ge=1, le=100),
    organization_id: Optional[int] = Query(None, description="Organization ID"),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """List knowledge bases accessible to the current user"""
    service = KnowledgeBaseService(db)
    result = await service.list_knowledge_bases(
        user_id=current_user.id,
        page_num=page_num,
        page_size=page_size,
        organization_id=organization_id
    )
    return resp_200(result.data)

@router.get("/{kb_id}",
    summary="Get knowledge base")
async def get_knowledge_base(
    kb_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Get a knowledge base by ID"""
    service = KnowledgeBaseService(db)
    result = await service.get_knowledge_base(kb_id, current_user.id)
    return resp_200(result.data)

@router.put("/{kb_id}",
    summary="Update knowledge base")
async def update_knowledge_base(
    kb_id: int,
    kb_update: KnowledgeBaseUpdate,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Update a knowledge base"""
    service = KnowledgeBaseService(db)
    result = await service.update_knowledge_base(kb_id, kb_update, current_user.id)
    return resp_200(result.data)

@router.delete("/{kb_id}",
    summary="Delete knowledge base")
async def delete_knowledge_base(
    kb_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Delete a knowledge base"""
    service = KnowledgeBaseService(db)
    result = await service.delete_knowledge_base(kb_id, current_user.id)
    return resp_200(result.data)


@router.get("/{kb_id}/similar",
    response_model=KnowledgeBaseList,
    summary="Find similar knowledge bases",)
async def find_similar_knowledge_bases(
    kb_id: int,
    limit: int = Query(5, ge=1, le=20),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Find similar knowledge bases"""
    service = KnowledgeBaseService(db)
    return await service.find_similar(kb_id, limit, current_user.id)

@router.post("/{kb_id}/share",
    summary="Share knowledge base")
async def share_knowledge_base(
    kb_id: int,
    visibility: Visibility,
    organization_id: Optional[int] = None,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Share a knowledge base"""
    service = KnowledgeBaseService(db)
    return await service.share(kb_id, visibility, organization_id, current_user.id)

@router.post("/{kb_id}/clone", 
    response_model=KnowledgeBaseResponse,
    summary="Clone knowledge base")
async def clone_knowledge_base(
    kb_id: int,
    name: str,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Clone a knowledge base"""
    service = KnowledgeBaseService(db)
    return await service.clone(kb_id, name, current_user.id)

# Document Routes

@router.post("/{kb_id}/documents", 
    response_model=DocumentResponse,
    summary="Upload document")
async def upload_document(
    kb_id: int,
    file: UploadFile = File(...),
    metadata: Optional[DocumentUpload] = None,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Upload a document to a knowledge base"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.upload_document(kb_id, file, current_user.id, metadata)

@router.post("/{kb_id}/documents/batch", 
    response_model=List[DocumentResponse],
    summary="Batch upload documents")
async def batch_upload_documents(
    kb_id: int,
    files: List[UploadFile] = File(...),
    metadata: Optional[DocumentUpload] = None,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Batch upload documents to a knowledge base"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.batch_upload_documents(kb_id, files, current_user.id, metadata)

@router.get("/{kb_id}/documents", 
    response_model=DocumentList,
    summary="List documents")
async def list_documents(
    kb_id: int,
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    file_type: Optional[str] = None,
    status: Optional[str] = None,
    search: Optional[str] = None,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """List documents in a knowledge base"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    documents, total = await doc_service.list_documents(
        kb_id, current_user.id, skip, limit, file_type, status, search
    )
    return {"total": total, "items": documents}

@router.get("/documents/{doc_id}", 
    response_model=DocumentResponse,
    summary="Get document")
async def get_document(
    doc_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Get a document by ID"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.get_document(doc_id, current_user.id)

@router.put("/documents/{doc_id}", 
    response_model=DocumentResponse,
    summary="Update document")
async def update_document(
    doc_id: int,
    update_data: DocumentUpdate,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Update a document"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.update_document(doc_id, update_data, current_user.id)

@router.delete("/documents/{doc_id}",
    summary="Delete document")
async def delete_document(
    doc_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Delete a document"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    await doc_service.delete_document(doc_id, current_user.id)

@router.get("/documents/{doc_id}/chunks", 
    response_model=DocumentChunkList,
    summary="List document chunks")
async def list_document_chunks(
    doc_id: int,
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """List chunks of a document"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    chunks, total = await doc_service.list_chunks(doc_id, current_user.id, skip, limit)
    return {"total": total, "items": chunks}

@router.get("/documents/{doc_id}/similar", 
    response_model=DocumentList,
    summary="Find similar documents")
async def find_similar_documents(
    doc_id: int,
    limit: int = Query(5, ge=1, le=20),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Find similar documents"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.find_similar(doc_id, limit, current_user.id)

@router.post("/documents/{doc_id}/reprocess",
    summary="Reprocess document")
async def reprocess_document(
    doc_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Reprocess a document"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.reprocess_document(doc_id, current_user.id)

@router.get("/documents/{doc_id}/download",
    summary="Download original document")
async def download_document(
    doc_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Download original document"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.download_document(doc_id, current_user.id)

@router.get("/documents/{doc_id}/export",
    summary="Export processed document")
async def export_document(
    doc_id: int,
    format: str = Query(..., regex="^(json|txt|pdf)$"),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Export processed document"""
    kb_service = KnowledgeBaseService(db)
    doc_service = DocumentService(db, kb_service)
    return await doc_service.export_document(doc_id, format, current_user.id)

# Search Routes

@router.post("/{kb_id}/search", 
    response_model=SearchResponse,
    summary="Search knowledge base")
async def search_knowledge_base(
    kb_id: int,
    query: SearchQuery,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Search documents in a knowledge base"""
    service = KnowledgeBaseService(db)
    return await service.search(kb_id, query, current_user.id)

@router.post("/{kb_id}/search/similar", 
    response_model=SearchResponse,
    summary="Similarity search")
async def similarity_search(
    kb_id: int,
    document_id: int,
    limit: int = Query(5, ge=1, le=20),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Find similar content based on a document"""
    service = KnowledgeBaseService(db)
    return await service.similarity_search(kb_id, document_id, limit, current_user.id)

@router.post("/{kb_id}/search/semantic", 
    response_model=SearchResponse,
    summary="Semantic search")
async def semantic_search(
    kb_id: int,
    query: SearchQuery,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Semantic search in knowledge base"""
    service = KnowledgeBaseService(db)
    return await service.semantic_search(kb_id, query, current_user.id)

@router.post("/{kb_id}/search/hybrid", 
    response_model=SearchResponse,
    summary="Hybrid search")
async def hybrid_search(
    kb_id: int,
    query: SearchQuery,
    weights: Dict[str, float] = {"semantic": 0.7, "keyword": 0.3},
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user)
):
    """Hybrid search combining semantic and keyword search"""
    service = KnowledgeBaseService(db)
    return await service.hybrid_search(kb_id, query, weights, current_user.id)
