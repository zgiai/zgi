from sqlalchemy import Column, BigInteger, String, Text, Integer, DECIMAL, DateTime, JSON, SmallInteger, ForeignKey
from sqlalchemy.sql import func
from sqlalchemy.orm import relationship
from app.db.base_class import Base

class ChatSession(Base):
    __tablename__ = "chat_sessions"

    id = Column(BigInteger, primary_key=True, autoincrement=True, index=True, comment="Unique record ID")
    user_id = Column(BigInteger, nullable=False, comment="User ID for distinguishing different users' conversations")
    conversation_id = Column(String(100), nullable=False, comment="Unique conversation identifier for multi-turn dialogues")
    request_id = Column(String(100), comment="Unique request identifier for logging and tracing")
    
    question = Column(Text, comment="User input content")
    answer = Column(Text, comment="Model generated response")
    
    model = Column(String(100), nullable=False, comment="Model name and version used, e.g., gpt-4, gpt-3.5-turbo")
    
    prompt_tokens = Column(Integer, nullable=False, default=0, comment="Number of tokens consumed by user input")
    completion_tokens = Column(Integer, nullable=False, default=0, comment="Number of tokens consumed by model response")
    cost = Column(DECIMAL(11,7), nullable=False, default=0.0, comment="Cost of this conversation in USD")
    
    api_key = Column(String(200), comment="API Key used for the call (if any)")
    interaction_type = Column(SmallInteger, nullable=False, default=1, comment="Interaction type: 1-text chat, 2-code generation")
    
    app_id = Column(Integer, comment="Application ID to distinguish different application requests")
    source = Column(SmallInteger, comment="Request source: 1-Web, 2-Mobile, 3-API call")
    
    ip_address = Column(String(45), nullable=False, default="", comment="User request IP address (IPv4/IPv6)")
    
    is_violation = Column(SmallInteger, default=0, comment="Content violation flag: 0-no, 1-yes")
    status = Column(SmallInteger, nullable=False, default=1, comment="Request status: 1-success, 2-failure, 3-other")
    
    parameters = Column(JSON, comment="Chat request parameters (JSON), e.g., temperature, top_p")
    
    openai_response_id = Column(String(100), comment="OpenAI response ID, e.g., chatcmpl-xxx")
    openai_system_fingerprint = Column(String(100), comment="OpenAI system fingerprint")
    openai_created_at = Column(Integer, comment="OpenAI response created timestamp")
    
    raw_request = Column(JSON, comment="Original OpenAI API request parameters (JSON)")
    raw_response_chunks = Column(JSON, comment="Original OpenAI API streaming response data (JSON array)")
    finish_reason = Column(String(50), comment="Conversation end reason (stop, length...)")
    
    created_at = Column(DateTime, nullable=False, server_default=func.now(), comment="Record creation time")
    updated_at = Column(DateTime, nullable=False, server_default=func.now(), onupdate=func.now(), comment="Record update time")

    files = relationship("ChatFile", back_populates="session")

class ChatFile(Base):
    __tablename__ = "chat_files"

    id = Column(Integer, primary_key=True, index=True)
    session_id = Column(Integer, ForeignKey("chat_sessions.id"), nullable=False)
    filename = Column(String(255), nullable=False)
    content_type = Column(String(100), nullable=False)
    file_size = Column(Integer, nullable=False)
    file_path = Column(Text, nullable=False)
    created_at = Column(DateTime, nullable=False, server_default=func.now())
    deleted_at = Column(DateTime, nullable=True)

    session = relationship("ChatSession", back_populates="files")
