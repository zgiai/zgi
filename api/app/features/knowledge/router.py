from datetime import datetime
import json
from typing import Optional, Dict, Any, List
from fastapi import APIRouter, Depends, File, UploadFile, Query, HTTPException, Form, BackgroundTasks, Body
from pydantic import ValidationError
from sqlalchemy.orm import Session

from app.core.base import resp_200
from app.core.database import get_db, get_sync_db
from app.core.deps import get_current_user
from app.features import User
from app.features.knowledge.models.knowledge import Visibility, Status
from app.features.knowledge.schemas.request.knowledge import (
    KnowledgeBaseCreate,
    KnowledgeBaseUpdate,
    SearchQuery,
    GetVectorQuery
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


# dependencies
def get_knowledge_base_service(db: Session = Depends(get_sync_db)) -> KnowledgeBaseService:
    return KnowledgeBaseService(db)


def get_document_service(
        sync_db: Session = Depends(get_sync_db),
        kb_service: KnowledgeBaseService = Depends(get_knowledge_base_service)
) -> DocumentService:
    return DocumentService(sync_db, kb_service)


# Knowledge Base Routes

@router.post("/create",
             summary="Create knowledge base")
async def create_knowledge_base(
        kb_create: KnowledgeBaseCreate,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        current_user: User = Depends(get_current_user)
):
    """Create a new knowledge base"""
    result = await service.create_knowledge_base(kb_create, current_user.id)
    return resp_200(result.data)


@router.get("",
            summary="List knowledge bases")
async def list_knowledge_bases(
        page_num: int = Query(1, ge=1),
        page_size: int = Query(10, ge=1, le=100),
        organization_id: Optional[int] = Query(None, description="Organization ID"),
        query_name: Optional[str] = Query(None, description="Search by name"),
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        current_user: User = Depends(get_current_user)
):
    """List knowledge bases accessible to the current user"""
    result = await service.list_knowledge_bases(
        user_id=current_user.id,
        page_num=page_num,
        page_size=page_size,
        organization_id=organization_id,
        query_name=query_name
    )
    return resp_200(result.data)


@router.get("/{kb_id}",
            summary="Get knowledge base")
async def get_knowledge_base(
        kb_id: int,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        current_user: User = Depends(get_current_user)
):
    """Get a knowledge base by ID"""
    result = await service.get_knowledge_base(kb_id, current_user.id)
    return resp_200(result.data)


@router.put("/{kb_id}",
            summary="Update knowledge base")
async def update_knowledge_base(
        kb_id: int,
        kb_update: KnowledgeBaseUpdate,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        current_user: User = Depends(get_current_user)
):
    """Update a knowledge base"""
    result = await service.update_knowledge_base(kb_id, kb_update, current_user.id)
    return resp_200(result.data)


@router.delete("/{kb_id}",
               summary="Delete knowledge base")
async def delete_knowledge_base(
        kb_id: int,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Delete a knowledge base"""
    # result = await service.delete_knowledge_base(kb_id, current_user.id)
    result = await doc_service.delete_knowledge_base(kb_id, current_user.id)
    return resp_200(result.data)


@router.post("/{kb_id}/clone",
             summary="Clone knowledge base")
async def clone_knowledge_base(
        *,
        kb_id: int,
        name: Optional[str] = None,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        doc_service: DocumentService = Depends(get_document_service),
        current_user: User = Depends(get_current_user),
        background_tasks: BackgroundTasks
):
    """Clone a knowledge base"""
    # result = await service.clone_knowledge_base(kb_id, name, current_user.id)
    result = await doc_service.clone_knowledge_base(kb_id, name, current_user.id, background_tasks)
    return resp_200(result.data)


# Document Routes

@router.post("/{kb_id}/documents",
             summary="Upload document")
async def upload_document(
        *,
        kb_id: int,
        file: UploadFile = File(...),
        metadata: Optional[str] = Form(None),
        chunk_rule: Optional[str] = Form(None),
        doc_service: DocumentService = Depends(get_document_service),
        current_user: User = Depends(get_current_user),
        background_tasks: BackgroundTasks):
    """Upload a document to a knowledge base"""
    # Manually parse metadata JSON string to DocumentUpload object
    metadata_dict = None
    if metadata:
        try:
            metadata_dict = json.loads(metadata)
        except (json.JSONDecodeError, ValidationError) as e:
            raise ValidationError(f"Invalid metadata: {e}")
    # Manually parse chunk_rule JSON string to dict
    chunk_rule_dict = None
    if chunk_rule:
        try:
            chunk_rule_dict = json.loads(chunk_rule)
        except (json.JSONDecodeError, ValidationError) as e:
            raise ValidationError(f"Invalid chunk_rule: {e}")
    return await doc_service.upload_document(kb_id, file, current_user.id, background_tasks, metadata_dict, chunk_rule_dict)


@router.post("/{kb_id}/documents/batch",
             summary="Batch upload documents")
async def batch_upload_documents(
        *,
        kb_id: int,
        files: List[UploadFile] = File(...),
        metadata: Optional[str] = Form(None),
        chunk_rule: Optional[str] = Form(None),
        db: Session = Depends(get_db),
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user),
        background_tasks: BackgroundTasks
):
    """Batch upload documents to a knowledge base"""
    metadata_dict = None
    if metadata:
        try:
            metadata_dict = json.loads(metadata)
        except (json.JSONDecodeError, ValidationError) as e:
            raise ValidationError(f"Invalid metadata: {e}")
    # Manually parse chunk_rule JSON string to dict
    chunk_rule_dict = None
    if chunk_rule:
        try:
            chunk_rule_dict = json.loads(chunk_rule)
        except (json.JSONDecodeError, ValidationError) as e:
            raise ValidationError(f"Invalid chunk_rule: {e}")
    return await doc_service.batch_upload_documents(kb_id, files, current_user.id, background_tasks, metadata_dict, chunk_rule_dict)


@router.get("/{kb_id}/documents",
            summary="List documents")
async def list_documents(
        kb_id: int,
        page_num: int = Query(1, ge=1),
        page_size: int = Query(10, ge=1, le=100),
        file_type: Optional[str] = None,
        status: Optional[str] = None,
        search: Optional[str] = None,
        doc_service: DocumentService = Depends(get_document_service),
        current_user: User = Depends(get_current_user)
):
    """List documents in a knowledge base"""
    # kb_service = KnowledgeBaseService(db)
    # doc_service = DocumentService(sync_db, kb_service)
    documents, total = await doc_service.list_documents(
        kb_id, current_user.id, page_num, page_size, file_type, status, search
    )
    document_resp = [DocumentResponse.model_validate(doc) for doc in documents]
    data_list = DocumentList(total=total, items=document_resp)
    return resp_200(data_list)


@router.get("/documents/{doc_id}",
            summary="Get document")
async def get_document(
        doc_id: int,
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Get a document by ID"""
    document = await doc_service.get_document(doc_id, current_user.id)
    resp_data = DocumentResponse.model_validate(document)
    return resp_200(resp_data)


@router.put("/documents/{doc_id}",
            summary="Update document")
async def update_document(
        doc_id: int,
        update_data: DocumentUpdate,
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Update a document"""
    document = await doc_service.update_document(doc_id, update_data, current_user.id)
    resp_data = DocumentResponse.model_validate(document)
    return resp_200(resp_data)


@router.delete("/documents/{doc_id}",
               summary="Delete document")
async def delete_document(
        doc_id: int,
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Delete a document"""
    await doc_service.delete_document(doc_id, current_user.id)
    return resp_200("Document deleted")


@router.get("/documents/{doc_id}/chunks",
            summary="List document chunks")
async def list_document_chunks(
        doc_id: int,
        page_num: int = Query(1, ge=1),
        page_size: int = Query(10, ge=1, le=100),
        search: Optional[str] = Query(None, description="Search by content or chunk_meta_info"),
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """List chunks of a document"""
    chunks, total = await doc_service.list_chunks(doc_id, current_user.id, page_num, page_size, search)
    chunk_resp = [DocumentChunkResponse.model_validate(chunk) for chunk in chunks]
    data_list = DocumentChunkList(total=total, items=chunk_resp)
    return resp_200(data_list)


@router.get("/documents/chunks/{chunk_id}", summary="Get document chunk by ID")
async def get_document_chunk(
        chunk_id: int,
        doc_service: DocumentService = Depends(get_document_service),
        current_user: User = Depends(get_current_user)
):
    """Get a document chunk by ID"""
    chunk = await doc_service.get_chunk(chunk_id, current_user.id)
    resp_data = DocumentChunkResponse.model_validate(chunk)
    return resp_200(resp_data)


@router.put("/documents/chunks/{chunk_id}", summary="Update document chunk by ID")
async def update_document_chunk(
        chunk_id: int,
        content: str = Body(..., embed=True),
        doc_service: DocumentService = Depends(get_document_service),
        current_user: User = Depends(get_current_user)
):
    """Update a document chunk by ID"""
    try:
        chunk = await doc_service.update_chunk(chunk_id, content, current_user.id)
        resp_data = DocumentChunkResponse.model_validate(chunk)
        return resp_200(resp_data)
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/documents/{doc_id}/reprocess",
             summary="Reprocess document")
async def reprocess_document(
        *,
        doc_id: int,
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user),
        background_tasks: BackgroundTasks
):
    """Reprocess a document"""
    return await doc_service.reprocess_document(doc_id, current_user.id, background_tasks)


@router.get("/documents/{doc_id}/download",
            summary="Download original document")
async def download_document(
        doc_id: int,
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Download original document"""
    return await doc_service.download_document(doc_id, current_user.id)


# Search Routes

@router.post("/{kb_id}/search_vector",
             summary="Search knowledge base vector")
async def search_knowledge_base(
        kb_id: int,
        query: SearchQuery,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        current_user=Depends(get_current_user)
):
    """Search documents in a knowledge base"""
    results = await service.search(kb_id, query, current_user.id)
    return resp_200(results)


@router.post("/{kb_id}/search",
             summary="Search knowledge base with document info")
async def search_knowledge_base_with_document_info(
        kb_id: int,
        query: SearchQuery,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Search documents in a knowledge base with additional document info"""
    results = await service.search(kb_id, query, current_user.id)
    document_ids = [result['document_id'] for result in results]
    documents = await doc_service.get_documents_by_ids(document_ids)
    document_info_map = {doc.id: doc for doc in documents}
    
    for result in results:
        doc_id = result['document_id']
        if doc_id in document_info_map:
            doc_file_name = ""
            doc_title = ""
            if document_info_map.get(doc_id) is not None:
                doc_file_name = document_info_map[doc_id].file_name
                doc_title = document_info_map[doc_id].title
            result['document_file_name'] = doc_file_name
            result['document_title'] = doc_title
            # Add more document fields as needed
    return resp_200(results)


@router.post("/{kb_id}/get_vectors",
             summary="Search knowledge base")
async def get_vectors(
        kb_id: int,
        query: GetVectorQuery,
        service: KnowledgeBaseService = Depends(get_knowledge_base_service),
        doc_service: DocumentService = Depends(get_document_service),
        current_user=Depends(get_current_user)
):
    """Search documents in a knowledge base"""
    # results = await service.search(kb_id, query)
    results = await doc_service.get_vectors(kb_id, query)
    return resp_200(results)
