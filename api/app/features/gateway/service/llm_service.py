"""Service for handling LLM requests"""
import uuid
from typing import Dict, Any, AsyncGenerator
from sqlalchemy.orm import Session
from ..schemas.chat import AddChatMessagesRequest
import logging

from ..providers.router import LLMRouter, get_router
from ... import User
from ...chat.models.conversation import Conversation, ChatMessage

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class LLMService:
    """Service for handling LLM requests"""
    
    _router = None
    
    @classmethod
    def get_router(cls) -> LLMRouter:
        """Get or create the LLM router instance."""
        if cls._router is None:
            cls._router = get_router()
        return cls._router
    
    @classmethod
    async def create_chat_completion(
        cls,
        params: Dict[str, Any],
        db: Session
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Create a chat completion.
        
        Args:
            params: Request parameters
            db: Database session
            
        Yields:
            Response chunks
            
        Raises:
            ValueError: If required parameters are missing
        """
        service = cls()
        
        # Extract required parameters
        messages = params.get("messages")
        if not messages:
            raise ValueError("messages parameter is required")
            
        model = params.get("model")
        if not model:
            raise ValueError("model parameter is required")
            
        # Extract optional parameters
        temperature = params.get("temperature", 1.0)
        max_tokens = params.get("max_tokens")
        stream = params.get("stream", False)
        
        # Remove processed params and pass rest as kwargs
        known_params = {"messages", "model", "temperature", "max_tokens", "stream", "api_key"}
        kwargs = {k: v for k, v in params.items() if k not in known_params}
        
        async for chunk in service.chat_completion(
            messages=messages,
            model=model,
            temperature=temperature,
            max_tokens=max_tokens,
            stream=stream,
            **kwargs
        ):
            yield chunk
    
    async def chat_completion(
        self,
        messages: list[Dict[str, Any]],
        model: str,
        temperature: float = 1.0,
        max_tokens: int = None,
        stream: bool = False,
        **kwargs: Any
    ) -> AsyncGenerator[Dict[str, Any], None]:
        """Generate chat completion using the appropriate provider.
        
        Args:
            messages: List of messages
            model: Model name
            temperature: Temperature value
            max_tokens: Maximum tokens to generate
            stream: Whether to stream responses
            **kwargs: Additional arguments
            
        Yields:
            Response chunks
        """
        router = self.get_router()
        
        try:
            async for chunk in router.chat_completion(
                messages=messages,
                model=model,
                temperature=temperature,
                max_tokens=max_tokens,
                stream=stream,
                **kwargs
            ):
                yield chunk
        except Exception as e:
            logger.error(f"Chat completion failed: {str(e)}")
            raise

    @staticmethod
    def add_chat_messages(request: AddChatMessagesRequest, user: User, db: Session):
        session_id = request.session_id
        messages = request.messages

        # Retrieve or create conversation
        user_id = user.id if user else None
        if session_id:
            conversation = db.query(Conversation).filter_by(session_id=session_id).first()
            if not conversation:
                raise ValueError(f"Conversation {session_id} not found")
            if conversation.user_id != user_id:
                raise ValueError("User does not have access to this conversation")
        else:
            session_id = str(uuid.uuid4())
            conversation = Conversation(session_id=session_id, user_id=user_id)
            db.add(conversation)
            db.commit()
            db.refresh(conversation)
        print(f"Session ID: {session_id}")
        print(f"Conversation ID: {conversation.id}")
        # Save each message to history
        for msg in messages:
            message = ChatMessage(
                conversation_id=conversation.id,
                role=msg.role,
                content=msg.content
            )
            db.add(message)
        db.commit()
        return conversation
