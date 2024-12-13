import json
import asyncio
import openai
import tiktoken
from typing import AsyncGenerator, Optional, List, Tuple
from sqlalchemy.orm import Session
from sqlalchemy import desc, and_, or_
from fastapi import HTTPException, status
from datetime import datetime

from app.core.config import settings
from app.features.users.models import User
from app.features.applications.models import Application
from app.features.chat.models import ChatSession
from app.features.chat.schemas import ChatMessage, SwitchModelResponse, ChatHistoryParams, ChatHistoryResponse

class ChatService:
    def __init__(self, db: Session):
        self.db = db
        openai.api_key = settings.OPENAI_API_KEY

    def count_tokens(self, text: str, model: str) -> int:
        """Count tokens for a given text using the specified model"""
        try:
            encoding = tiktoken.encoding_for_model(model)
            return len(encoding.encode(text))
        except Exception:
            # Fallback to approximate token count if model not found
            return len(text.split()) * 1.3

    def generate_title(self, messages: List[ChatMessage]) -> str:
        """Generate a title for the chat session based on the first user message"""
        if not messages:
            return "New Chat"
        
        first_msg = next((msg for msg in messages if msg.role == "user"), None)
        if not first_msg:
            return "New Chat"
        
        # Use the first few words of the message as title
        words = first_msg.content.split()[:5]
        title = " ".join(words)
        return f"{title}..." if len(words) == 5 else title

    def create_session(
        self,
        user_id: int,
        model: str,
        application_id: Optional[int] = None,
        title: Optional[str] = None,
        tags: Optional[List[str]] = None
    ) -> ChatSession:
        """Create a new chat session"""
        session = ChatSession(
            user_id=user_id,
            application_id=application_id,
            model=model,
            title=title,
            tags=tags or [],
            messages=[],
            message_count=0,
            total_tokens=0,
            last_message_at=datetime.utcnow()
        )
        self.db.add(session)
        self.db.commit()
        self.db.refresh(session)
        return session

    def get_session(self, session_id: int, user_id: int) -> Optional[ChatSession]:
        """Get a chat session by ID"""
        return self.db.query(ChatSession).filter(
            ChatSession.id == session_id,
            ChatSession.user_id == user_id,
            ChatSession.is_archived == False
        ).first()

    def update_session(
        self,
        session: ChatSession,
        messages: List[ChatMessage],
        model: Optional[str] = None
    ) -> ChatSession:
        """Update a chat session"""
        # Update messages and count
        session.messages = [msg.dict() for msg in messages]
        session.message_count = len(messages)
        session.last_message_at = datetime.utcnow()
        
        # Update model if provided
        if model:
            session.model = model
        
        # Update token count
        session.total_tokens = sum(
            self.count_tokens(msg.content, session.model)
            for msg in messages
        )
        
        # Generate or update title if not set
        if not session.title:
            session.title = self.generate_title(messages)
        
        # Update timestamps
        session.updated_at = datetime.utcnow()
        
        self.db.commit()
        self.db.refresh(session)
        return session

    def get_chat_history(
        self,
        user_id: int,
        params: ChatHistoryParams
    ) -> ChatHistoryResponse:
        """Get paginated chat history for a user"""
        # Base query
        query = self.db.query(ChatSession).filter(
            ChatSession.user_id == user_id,
            ChatSession.is_archived == False
        )

        # Apply filters
        if params.start_date:
            query = query.filter(ChatSession.created_at >= params.start_date)
        if params.end_date:
            query = query.filter(ChatSession.created_at <= params.end_date)
        if params.model:
            query = query.filter(ChatSession.model == params.model)
        if params.search_term:
            search = f"%{params.search_term}%"
            query = query.filter(or_(
                ChatSession.title.ilike(search),
                ChatSession.summary.ilike(search)
            ))
        if params.tags:
            # Filter sessions that have any of the specified tags
            query = query.filter(ChatSession.tags.overlap(params.tags))

        # Get total count
        total = query.count()

        # Apply pagination and ordering
        query = query.order_by(
            desc(ChatSession.last_message_at),
            desc(ChatSession.updated_at)
        )
        offset = (params.page - 1) * params.page_size
        sessions = query.offset(offset).limit(params.page_size).all()

        return ChatHistoryResponse(
            total=total,
            page=params.page,
            page_size=params.page_size,
            sessions=sessions
        )

    def archive_session(self, session_id: int, user_id: int) -> bool:
        """Archive (soft delete) a chat session"""
        session = self.get_session(session_id, user_id)
        if not session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Chat session not found"
            )

        session.is_archived = True
        self.db.commit()
        return True

    def delete_session(self, session_id: int, user_id: int) -> bool:
        """Hard delete a chat session"""
        session = self.get_session(session_id, user_id)
        if not session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Chat session not found"
            )

        self.db.delete(session)
        self.db.commit()
        return True

    def add_tags(self, session_id: int, user_id: int, tags: List[str]) -> ChatSession:
        """Add tags to a chat session"""
        session = self.get_session(session_id, user_id)
        if not session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Chat session not found"
            )

        # Add new tags without duplicates
        current_tags = set(session.tags)
        current_tags.update(tags)
        session.tags = list(current_tags)
        
        self.db.commit()
        self.db.refresh(session)
        return session

    def remove_tags(self, session_id: int, user_id: int, tags: List[str]) -> ChatSession:
        """Remove tags from a chat session"""
        session = self.get_session(session_id, user_id)
        if not session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Chat session not found"
            )

        # Remove specified tags
        session.tags = [tag for tag in session.tags if tag not in tags]
        
        self.db.commit()
        self.db.refresh(session)
        return session

    async def stream_chat(
        self,
        messages: List[ChatMessage],
        model: str,
        temperature: float = 0.7,
        max_tokens: Optional[int] = None,
        user: Optional[User] = None,
        application: Optional[Application] = None,
        session_id: Optional[int] = None
    ) -> AsyncGenerator[str, None]:
        """Stream chat responses from OpenAI API"""
        try:
            # Get or create session
            session = None
            if session_id:
                session = self.get_session(session_id, user.id)
            if not session and user:
                session = self.create_session(
                    user_id=user.id,
                    model=model,
                    application_id=application.id if application else None
                )

            # Call OpenAI API
            response = await openai.ChatCompletion.acreate(
                model=model,
                messages=[{"role": msg.role, "content": msg.content} for msg in messages],
                temperature=temperature,
                max_tokens=max_tokens,
                stream=True
            )

            # Collect the full response
            full_response = ""
            async for chunk in response:
                if chunk and chunk.choices and chunk.choices[0].delta.content:
                    content = chunk.choices[0].delta.content
                    full_response += content
                    yield content

            # Update session with the new message
            if session:
                all_messages = messages + [ChatMessage(role="assistant", content=full_response)]
                self.update_session(session, all_messages)

        except openai.error.InvalidRequestError as e:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e)
            )
        except openai.error.AuthenticationError:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid OpenAI API key"
            )
        except Exception as e:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )
