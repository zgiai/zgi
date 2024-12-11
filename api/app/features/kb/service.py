import os
from typing import List, Optional, Tuple
from fastapi import UploadFile, HTTPException
import PyPDF2
from sqlalchemy.orm import Session

from app.models.knowledge_base import KnowledgeBase, Document, Visibility
from app.features.kb.schemas import KnowledgeBaseCreate, KnowledgeBaseUpdate, SearchQuery
from app.core.security import get_current_user
from app.features.rag.service import RAGService


class KnowledgeBaseService:
    def __init__(self, db: Session, rag_service: RAGService):
        self.db = db
        self.rag_service = rag_service

    def create_knowledge_base(
        self, kb_create: KnowledgeBaseCreate, user_id: int
    ) -> KnowledgeBase:
        kb = KnowledgeBase(
            name=kb_create.name,
            description=kb_create.description,
            visibility=kb_create.visibility,
            owner_id=user_id,
        )
        self.db.add(kb)
        self.db.commit()
        self.db.refresh(kb)
        return kb

    def get_knowledge_base(self, kb_id: int, user_id: int) -> Optional[KnowledgeBase]:
        kb = self.db.query(KnowledgeBase).filter(KnowledgeBase.id == kb_id).first()
        if not kb:
            return None
        if kb.visibility == Visibility.PRIVATE and kb.owner_id != user_id:
            return None
        return kb

    def list_knowledge_bases(
        self, user_id: int, skip: int = 0, limit: int = 10
    ) -> Tuple[List[KnowledgeBase], int]:
        query = self.db.query(KnowledgeBase).filter(
            (KnowledgeBase.visibility == Visibility.PUBLIC)
            | (KnowledgeBase.owner_id == user_id)
        )
        total = query.count()
        kbs = query.offset(skip).limit(limit).all()
        return kbs, total

    def update_knowledge_base(
        self, kb_id: int, kb_update: KnowledgeBaseUpdate, user_id: int
    ) -> Optional[KnowledgeBase]:
        kb = self.get_knowledge_base(kb_id, user_id)
        if not kb:
            return None
        if kb.owner_id != user_id:
            raise HTTPException(status_code=403, detail="Not authorized to update this knowledge base")
        
        update_data = kb_update.dict(exclude_unset=True)
        for field, value in update_data.items():
            setattr(kb, field, value)
        
        self.db.commit()
        self.db.refresh(kb)
        return kb

    def delete_knowledge_base(self, kb_id: int, user_id: int) -> bool:
        kb = self.get_knowledge_base(kb_id, user_id)
        if not kb:
            return False
        if kb.owner_id != user_id:
            raise HTTPException(status_code=403, detail="Not authorized to delete this knowledge base")
        
        self.db.delete(kb)
        self.db.commit()
        return True

    async def upload_document(
        self, kb_id: int, file: UploadFile, user_id: int
    ) -> Document:
        kb = self.get_knowledge_base(kb_id, user_id)
        if not kb:
            raise HTTPException(status_code=404, detail="Knowledge base not found")
        if kb.owner_id != user_id:
            raise HTTPException(status_code=403, detail="Not authorized to upload to this knowledge base")

        # Create document record
        document = Document(
            kb_id=kb_id,
            file_name=file.filename,
        )
        self.db.add(document)
        self.db.commit()
        self.db.refresh(document)

        try:
            # Process and vectorize document
            content = await self._process_document(file)
            document.file_content = content
            
            # Generate and store vectors
            vector_id = await self.rag_service.process_and_store_document(
                content, str(document.id)
            )
            document.vector_id = vector_id
            
            self.db.commit()
            return document
        except Exception as e:
            self.db.delete(document)
            self.db.commit()
            raise HTTPException(status_code=400, detail=f"Error processing document: {str(e)}")

    async def _process_document(self, file: UploadFile) -> str:
        content = ""
        if file.filename.endswith(".pdf"):
            pdf_content = await file.read()
            pdf_reader = PyPDF2.PdfReader(BytesIO(pdf_content))
            for page in pdf_reader.pages:
                content += page.extract_text() + "\n"
        elif file.filename.endswith(".txt"):
            content = (await file.read()).decode("utf-8")
        else:
            raise HTTPException(
                status_code=400, detail="Unsupported file format. Only PDF and TXT files are supported."
            )
        return content

    def list_documents(
        self, kb_id: int, user_id: int, skip: int = 0, limit: int = 10
    ) -> Tuple[List[Document], int]:
        kb = self.get_knowledge_base(kb_id, user_id)
        if not kb:
            raise HTTPException(status_code=404, detail="Knowledge base not found")

        query = self.db.query(Document).filter(Document.kb_id == kb_id)
        total = query.count()
        documents = query.offset(skip).limit(limit).all()
        return documents, total

    def delete_document(self, doc_id: int, user_id: int) -> bool:
        document = self.db.query(Document).filter(Document.id == doc_id).first()
        if not document:
            return False

        kb = self.get_knowledge_base(document.kb_id, user_id)
        if not kb or kb.owner_id != user_id:
            raise HTTPException(status_code=403, detail="Not authorized to delete this document")

        # Delete vector from vector database
        if document.vector_id:
            self.rag_service.delete_vectors([document.vector_id])

        self.db.delete(document)
        self.db.commit()
        return True

    async def search_documents(
        self, search_query: SearchQuery, user_id: int
    ) -> List[dict]:
        kb = self.get_knowledge_base(search_query.kb_id, user_id)
        if not kb:
            raise HTTPException(status_code=404, detail="Knowledge base not found")

        # Get document IDs in this knowledge base
        doc_ids = [
            str(doc.id)
            for doc in self.db.query(Document)
            .filter(Document.kb_id == search_query.kb_id)
            .all()
        ]

        if not doc_ids:
            return []

        # Perform vector search
        results = await self.rag_service.search(
            query=search_query.query,
            document_ids=doc_ids,
            top_k=search_query.top_k
        )

        # Enhance results with document information
        enhanced_results = []
        for result in results:
            document = self.db.query(Document).filter(
                Document.id == int(result["document_id"])
            ).first()
            if document:
                result["file_name"] = document.file_name
                enhanced_results.append(result)

        return enhanced_results
