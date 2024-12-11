import os
import hashlib
from datetime import datetime
from typing import List, Optional, Tuple, Dict, Any
from fastapi import UploadFile, HTTPException
from sqlalchemy.orm import Session
from sentence_transformers import SentenceTransformer
import PyPDF2
import pinecone

from app.models.rag import Document, QueryLog
from app.features.rag.schemas import (
    DocumentCreate,
    SearchRequest,
    GenerateRequest,
    QueryRequest,
    ChunkResult
)
from app.core.config import settings

class RAGService:
    def __init__(self, db: Session):
        self.db = db
        self.upload_dir = os.path.join(settings.UPLOAD_DIR, "rag_documents")
        os.makedirs(self.upload_dir, exist_ok=True)
        
        # Initialize embedding model
        self.embedding_model = SentenceTransformer('all-MiniLM-L6-v2')
        
        # Initialize Pinecone
        pinecone.init(
            api_key=settings.PINECONE_API_KEY,
            environment=settings.PINECONE_ENVIRONMENT
        )
        self.index = pinecone.Index(settings.PINECONE_INDEX_NAME)

    async def process_document(self, file: UploadFile, user_id: int, team_id: Optional[int] = None) -> Document:
        """Process and store a new document"""
        # Validate file type
        if file.content_type not in ["application/pdf", "text/plain"]:
            raise HTTPException(status_code=400, detail="Unsupported file type")

        # Read and hash file contents
        contents = await file.read()
        content_hash = hashlib.sha256(contents).hexdigest()

        # Check for duplicate
        existing_doc = self.db.query(Document).filter(
            Document.content_hash == content_hash,
            Document.user_id == user_id
        ).first()
        if existing_doc:
            raise HTTPException(status_code=400, detail="Document already exists")

        # Save file
        file_ext = os.path.splitext(file.filename)[1]
        file_path = os.path.join(self.upload_dir, f"{content_hash}{file_ext}")
        with open(file_path, "wb") as f:
            f.write(contents)

        # Create document record
        doc = Document(
            user_id=user_id,
            team_id=team_id,
            filename=file.filename,
            file_path=file_path,
            file_type=file.content_type,
            file_size=len(contents),
            content_hash=content_hash,
            status="processing"
        )
        self.db.add(doc)
        self.db.commit()

        try:
            # Extract text based on file type
            if file.content_type == "application/pdf":
                extracted_text = self._extract_pdf_text(file_path)
            else:  # text/plain
                extracted_text = contents.decode('utf-8')

            # Split into chunks and generate embeddings
            chunks = self._split_text(extracted_text)
            vectors = self._generate_embeddings(chunks)

            # Store vectors in Pinecone
            vector_ids = []
            for i, (chunk, vector) in enumerate(zip(chunks, vectors)):
                vector_id = f"{doc.id}_{i}"
                self.index.upsert([(vector_id, vector, {
                    "document_id": doc.id,
                    "chunk_index": i,
                    "text": chunk,
                    "metadata": doc.metadata
                })])
                vector_ids.append(vector_id)

            # Update document record
            doc.status = "completed"
            doc.extracted_text = extracted_text
            doc.vector_ids = vector_ids
            doc.chunk_count = len(chunks)
            doc.embedding_model = self.embedding_model.get_model_name()
            doc.processed_at = datetime.utcnow()
            self.db.commit()

        except Exception as e:
            doc.status = "failed"
            doc.error = str(e)
            self.db.commit()
            raise HTTPException(status_code=500, detail=f"Document processing failed: {str(e)}")

        return doc

    def _extract_pdf_text(self, file_path: str) -> str:
        """Extract text from PDF file"""
        text = []
        try:
            with open(file_path, 'rb') as file:
                pdf_reader = PyPDF2.PdfReader(file)
                for page in pdf_reader.pages:
                    text.append(page.extract_text())
            return "\n".join(text)
        except Exception as e:
            raise Exception(f"Failed to extract text from PDF: {str(e)}")

    def _split_text(self, text: str, chunk_size: int = 1000, overlap: int = 100) -> List[str]:
        """Split text into overlapping chunks"""
        chunks = []
        start = 0
        text_len = len(text)

        while start < text_len:
            end = start + chunk_size
            chunk = text[start:end]
            
            # Adjust chunk to end at sentence boundary if possible
            if end < text_len:
                last_period = chunk.rfind('.')
                if last_period != -1:
                    chunk = chunk[:last_period + 1]
                    end = start + last_period + 1

            chunks.append(chunk)
            start = end - overlap

        return chunks

    def _generate_embeddings(self, texts: List[str]) -> List[List[float]]:
        """Generate embeddings for text chunks"""
        return self.embedding_model.encode(texts).tolist()

    async def search(self, user_id: int, request: SearchRequest) -> List[ChunkResult]:
        """Search for relevant text chunks"""
        # Generate query embedding
        query_embedding = self.embedding_model.encode(request.query).tolist()

        # Prepare metadata filter
        metadata_filter = {}
        if request.document_ids:
            metadata_filter["document_id"] = {"$in": request.document_ids}
        if request.team_id:
            metadata_filter["team_id"] = request.team_id

        # Query vector database
        results = self.index.query(
            vector=query_embedding,
            top_k=request.top_k,
            filter=metadata_filter,
            include_metadata=True
        )

        # Convert to ChunkResult objects
        chunks = []
        for match in results.matches:
            if match.score < request.min_score:
                continue
            chunks.append(ChunkResult(
                text=match.metadata["text"],
                score=match.score,
                metadata=match.metadata.get("metadata", {}),
                document_id=match.metadata["document_id"],
                chunk_index=match.metadata["chunk_index"]
            ))

        return chunks

    async def generate(self, user_id: int, request: GenerateRequest) -> Dict[str, Any]:
        """Generate response using context chunks"""
        # Prepare context
        context = "\n\n".join([chunk.text for chunk in request.context_chunks])
        
        # Prepare prompt
        prompt = f"""Use the following context to answer the question. If the answer cannot be derived from the context, say so.

Context:
{context}

Question: {request.query}

Answer:"""

        # Call language model (implementation depends on your setup)
        # This is a placeholder - replace with actual LLM call
        response = await self._call_language_model(
            prompt,
            model=request.model,
            temperature=request.temperature,
            max_tokens=request.max_tokens
        )

        return {
            "response": response["text"],
            "tokens_used": response["tokens_used"],
            "model_used": response["model"],
            "duration_ms": response["duration_ms"],
            "metadata": response.get("metadata", {})
        }

    async def query(self, user_id: int, request: QueryRequest) -> Dict[str, Any]:
        """Combined search and generate functionality"""
        # Search for relevant chunks
        search_request = SearchRequest(
            query=request.query,
            document_ids=request.document_ids,
            team_id=request.team_id,
            top_k=request.top_k,
            min_score=request.min_score
        )
        chunks = await self.search(user_id, search_request)

        # Generate response
        generate_request = GenerateRequest(
            query=request.query,
            context_chunks=chunks,
            document_ids=request.document_ids,
            model=request.model,
            temperature=request.temperature,
            max_tokens=request.max_tokens
        )
        response = await self.generate(user_id, generate_request)

        # Log query
        log = QueryLog(
            user_id=user_id,
            query_text=request.query,
            retrieved_chunks=[{
                "chunk_index": chunk.chunk_index,
                "document_id": chunk.document_id,
                "score": chunk.score
            } for chunk in chunks],
            response_text=response["response"],
            tokens_used=response["tokens_used"],
            duration_ms=response["duration_ms"],
            metadata={
                "model_used": response["model_used"],
                **response["metadata"]
            }
        )
        self.db.add(log)
        self.db.commit()

        return {
            "query": request.query,
            "response": response["response"],
            "context_chunks": chunks,
            "tokens_used": response["tokens_used"],
            "model_used": response["model_used"],
            "duration_ms": response["duration_ms"],
            "metadata": response["metadata"]
        }

    async def _call_language_model(
        self,
        prompt: str,
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: Optional[int] = None
    ) -> Dict[str, Any]:
        """Call language model API - implement based on your LLM setup"""
        # This is a placeholder - implement actual LLM call
        raise NotImplementedError("Language model integration not implemented")
