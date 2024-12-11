from fastapi import APIRouter, Depends, HTTPException, UploadFile, File
from sqlalchemy.orm import Session
from typing import Optional

from app.core.database import get_db
from app.core.auth import get_current_user
from app.models.user import User
from .service import RAGService
from .schemas import (
    DocumentResponse,
    DocumentListParams,
    DocumentListResponse,
    SearchRequest,
    SearchResponse,
    GenerateRequest,
    GenerateResponse,
    QueryRequest,
    QueryResponse
)

router = APIRouter(prefix="/v1/rag", tags=["rag"])

@router.post("/upload", response_model=DocumentResponse)
async def upload_document(
    file: UploadFile = File(...),
    team_id: Optional[int] = None,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Upload and process a document for RAG"""
    service = RAGService(db)
    return await service.process_document(file, current_user.id, team_id)

@router.get("/documents", response_model=DocumentListResponse)
async def list_documents(
    params: DocumentListParams = Depends(),
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """List uploaded documents"""
    query = db.query(Document).filter(Document.user_id == current_user.id)
    
    if params.status:
        query = query.filter(Document.status == params.status)
    if params.file_type:
        query = query.filter(Document.file_type == params.file_type)
    if params.team_id:
        query = query.filter(Document.team_id == params.team_id)

    total = query.count()
    documents = query.offset((params.page - 1) * params.page_size)\
                    .limit(params.page_size)\
                    .all()

    return DocumentListResponse(
        items=documents,
        total=total,
        page=params.page,
        page_size=params.page_size
    )

@router.get("/documents/{document_id}", response_model=DocumentResponse)
async def get_document(
    document_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Get document details"""
    document = db.query(Document).filter(
        Document.id == document_id,
        Document.user_id == current_user.id
    ).first()
    
    if not document:
        raise HTTPException(status_code=404, detail="Document not found")
    
    return document

@router.delete("/documents/{document_id}")
async def delete_document(
    document_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Delete a document and its vectors"""
    document = db.query(Document).filter(
        Document.id == document_id,
        Document.user_id == current_user.id
    ).first()
    
    if not document:
        raise HTTPException(status_code=404, detail="Document not found")

    # Delete vectors from Pinecone
    service = RAGService(db)
    if document.vector_ids:
        service.index.delete(ids=document.vector_ids)

    # Delete file
    try:
        os.remove(document.file_path)
    except OSError:
        pass  # File might not exist

    # Delete database record
    db.delete(document)
    db.commit()

    return {"message": "Document deleted successfully"}

@router.post("/search", response_model=SearchResponse)
async def search_documents(
    request: SearchRequest,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Search for relevant text chunks"""
    service = RAGService(db)
    start_time = datetime.utcnow()
    
    results = await service.search(current_user.id, request)
    
    duration = (datetime.utcnow() - start_time).total_seconds() * 1000
    
    return SearchResponse(
        query=request.query,
        results=results,
        total_chunks=len(results),
        duration_ms=int(duration)
    )

@router.post("/generate", response_model=GenerateResponse)
async def generate_response(
    request: GenerateRequest,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Generate response using context chunks"""
    service = RAGService(db)
    start_time = datetime.utcnow()
    
    response = await service.generate(current_user.id, request)
    
    duration = (datetime.utcnow() - start_time).total_seconds() * 1000
    response["duration_ms"] = int(duration)
    
    return GenerateResponse(**response)

@router.post("/query", response_model=QueryResponse)
async def query_documents(
    request: QueryRequest,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db)
):
    """Combined search and generate functionality"""
    service = RAGService(db)
    start_time = datetime.utcnow()
    
    response = await service.query(current_user.id, request)
    
    duration = (datetime.utcnow() - start_time).total_seconds() * 1000
    response["duration_ms"] = int(duration)
    
    return QueryResponse(**response)
