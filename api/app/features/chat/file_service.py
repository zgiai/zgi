import os
import hashlib
from typing import Optional, BinaryIO
import PyPDF2
from fastapi import UploadFile, HTTPException
from sqlalchemy.orm import Session

from app.features.chat.models import ChatFile
from app.features.chat.file_schemas import ChatFileCreate
from app.core.config import settings

class ChatFileService:
    def __init__(self, db: Session):
        self.db = db
        self.upload_dir = os.path.join(settings.UPLOAD_DIR, "chat_files")
        os.makedirs(self.upload_dir, exist_ok=True)

    async def save_file(self, file: UploadFile, user_id: int, session_id: int) -> ChatFile:
        """Save uploaded file and create database record"""
        if not file.content_type == "application/pdf":
            raise HTTPException(status_code=400, detail="Only PDF files are supported")
        
        if file.size > settings.MAX_UPLOAD_SIZE:
            raise HTTPException(status_code=400, detail="File size exceeds maximum limit")

        # Generate unique filename
        content_hash = await self._calculate_hash(file)
        file_ext = os.path.splitext(file.filename)[1]
        unique_filename = f"{content_hash}{file_ext}"
        file_path = os.path.join(self.upload_dir, unique_filename)

        # Save file to disk
        contents = await file.read()
        with open(file_path, "wb") as f:
            f.write(contents)

        # Extract text from PDF
        extracted_text = self._extract_pdf_text(file_path)
        
        # Create database record
        file_data = ChatFileCreate(
            user_id=user_id,
            session_id=session_id,
            filename=file.filename,
            file_path=file_path,
            file_size=file.size,
            mime_type=file.content_type,
            content_hash=content_hash,
            extracted_text=extracted_text,
            metadata={"page_count": self._get_pdf_page_count(file_path)}
        )

        db_file = ChatFile(**file_data.model_dump())
        self.db.add(db_file)
        self.db.commit()
        self.db.refresh(db_file)
        
        return db_file

    async def _calculate_hash(self, file: UploadFile) -> str:
        """Calculate SHA-256 hash of file contents"""
        sha256_hash = hashlib.sha256()
        contents = await file.read()
        sha256_hash.update(contents)
        await file.seek(0)
        return sha256_hash.hexdigest()

    def _extract_pdf_text(self, file_path: str) -> str:
        """Extract text content from PDF file"""
        text = []
        try:
            with open(file_path, 'rb') as file:
                pdf_reader = PyPDF2.PdfReader(file)
                for page in pdf_reader.pages:
                    text.append(page.extract_text())
            return "\n".join(text)
        except Exception as e:
            raise HTTPException(status_code=400, detail=f"Failed to extract text from PDF: {str(e)}")

    def _get_pdf_page_count(self, file_path: str) -> int:
        """Get the number of pages in a PDF file"""
        try:
            with open(file_path, 'rb') as file:
                pdf_reader = PyPDF2.PdfReader(file)
                return len(pdf_reader.pages)
        except Exception:
            return 0

    def get_file(self, file_id: int, user_id: int) -> Optional[ChatFile]:
        """Retrieve file by ID and user ID"""
        return self.db.query(ChatFile).filter(
            ChatFile.id == file_id,
            ChatFile.user_id == user_id
        ).first()

    def delete_file(self, file_id: int, user_id: int) -> bool:
        """Delete file and its database record"""
        file = self.get_file(file_id, user_id)
        if not file:
            return False

        # Delete physical file
        try:
            os.remove(file.file_path)
        except OSError:
            pass  # File might not exist

        # Delete database record
        self.db.delete(file)
        self.db.commit()
        return True
