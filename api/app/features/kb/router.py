from typing import List
from fastapi import APIRouter, Depends, File, UploadFile, Query, HTTPException
from sqlalchemy.orm import Session

from app.core.deps import get_db, get_current_user
from app.features.kb.schemas import (
    KnowledgeBaseCreate,
    KnowledgeBaseUpdate,
    KnowledgeBaseResponse,
    KnowledgeBaseList,
    DocumentResponse,
    DocumentList,
    SearchQuery,
    SearchResponse,
)
from app.features.kb.service import KnowledgeBaseService
from app.features.rag.service import RAGService

router = APIRouter()


@router.post("/create", response_model=KnowledgeBaseResponse)
async def create_knowledge_base(
    kb_create: KnowledgeBaseCreate,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Create a new knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    kb = service.create_knowledge_base(kb_create, current_user.id)
    return kb


@router.get("/list", response_model=KnowledgeBaseList)
async def list_knowledge_bases(
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """List knowledge bases accessible to the current user."""
    service = KnowledgeBaseService(db, rag_service)
    kbs, total = service.list_knowledge_bases(current_user.id, skip, limit)
    return {"total": total, "items": kbs}


@router.get("/{kb_id}", response_model=KnowledgeBaseResponse)
async def get_knowledge_base(
    kb_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Get a specific knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    kb = service.get_knowledge_base(kb_id, current_user.id)
    if not kb:
        raise HTTPException(status_code=404, detail="Knowledge base not found")
    return kb


@router.put("/{kb_id}", response_model=KnowledgeBaseResponse)
async def update_knowledge_base(
    kb_id: int,
    kb_update: KnowledgeBaseUpdate,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Update a knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    kb = service.update_knowledge_base(kb_id, kb_update, current_user.id)
    if not kb:
        raise HTTPException(status_code=404, detail="Knowledge base not found")
    return kb


@router.delete("/{kb_id}")
async def delete_knowledge_base(
    kb_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Delete a knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    success = service.delete_knowledge_base(kb_id, current_user.id)
    if not success:
        raise HTTPException(status_code=404, detail="Knowledge base not found")
    return {"status": "success"}


@router.post("/upload", response_model=DocumentResponse)
async def upload_document(
    kb_id: int,
    file: UploadFile = File(...),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Upload a document to a knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    document = await service.upload_document(kb_id, file, current_user.id)
    return document


@router.get("/{kb_id}/documents", response_model=DocumentList)
async def list_documents(
    kb_id: int,
    skip: int = Query(0, ge=0),
    limit: int = Query(10, ge=1, le=100),
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """List documents in a knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    documents, total = service.list_documents(kb_id, current_user.id, skip, limit)
    return {"total": total, "items": documents}


@router.delete("/document/{doc_id}")
async def delete_document(
    doc_id: int,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Delete a document from a knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    success = service.delete_document(doc_id, current_user.id)
    if not success:
        raise HTTPException(status_code=404, detail="Document not found")
    return {"status": "success"}


@router.post("/search", response_model=SearchResponse)
async def search_documents(
    search_query: SearchQuery,
    db: Session = Depends(get_db),
    current_user = Depends(get_current_user),
    rag_service: RAGService = Depends(RAGService),
):
    """Search documents in a knowledge base."""
    service = KnowledgeBaseService(db, rag_service)
    results = await service.search_documents(search_query, current_user.id)
    return {"results": results}
