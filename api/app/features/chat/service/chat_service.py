from datetime import datetime
from typing import List, Optional
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select
from app.features.chat.models.chat import ChatSession, ChatFile

class ChatService:
    def __init__(self, db: AsyncSession):
        self.db = db

    async def create_session(self, user_id: int, model_id: int) -> ChatSession:
        """Create a new chat session"""
        session = ChatSession(
            user_id=user_id,
            model_id=model_id,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow()
        )
        self.db.add(session)
        await self.db.commit()
        await self.db.refresh(session)
        return session

    async def get_session(self, session_id: int) -> Optional[ChatSession]:
        """Get a chat session by ID"""
        result = await self.db.execute(
            select(ChatSession).filter(
                ChatSession.id == session_id,
                ChatSession.deleted_at.is_(None)
            )
        )
        return result.scalar_one_or_none()

    async def list_user_sessions(self, user_id: int) -> List[ChatSession]:
        """List all chat sessions for a user"""
        result = await self.db.execute(
            select(ChatSession).filter(
                ChatSession.user_id == user_id,
                ChatSession.deleted_at.is_(None)
            )
        )
        return list(result.scalars().all())

    async def delete_session(self, session_id: int) -> None:
        """Soft delete a chat session"""
        session = await self.get_session(session_id)
        if session:
            session.deleted_at = datetime.utcnow()
            await self.db.commit()

    async def upload_file(
        self,
        session_id: int,
        filename: str,
        content_type: str,
        file_size: int,
        file_path: str
    ) -> ChatFile:
        """Upload a file to a chat session"""
        file = ChatFile(
            session_id=session_id,
            filename=filename,
            content_type=content_type,
            file_size=file_size,
            file_path=file_path,
            created_at=datetime.utcnow()
        )
        self.db.add(file)
        await self.db.commit()
        await self.db.refresh(file)
        return file

    async def get_file(self, file_id: int) -> Optional[ChatFile]:
        """Get a chat file by ID"""
        result = await self.db.execute(
            select(ChatFile).filter(
                ChatFile.id == file_id,
                ChatFile.deleted_at.is_(None)
            )
        )
        return result.scalar_one_or_none()

    async def list_session_files(self, session_id: int) -> List[ChatFile]:
        """List all files in a chat session"""
        result = await self.db.execute(
            select(ChatFile).filter(
                ChatFile.session_id == session_id,
                ChatFile.deleted_at.is_(None)
            )
        )
        return list(result.scalars().all())

    async def delete_file(self, file_id: int) -> None:
        """Soft delete a chat file"""
        file = await self.get_file(file_id)
        if file:
            file.deleted_at = datetime.utcnow()
            await self.db.commit()
