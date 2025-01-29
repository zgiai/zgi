from sqlalchemy import Column, String, Float, Integer, DateTime, JSON
from sqlalchemy.sql import func
from app.core.database import Base


class Assistant(Base):
    id = Column(String(32), primary_key=True, index=True)
    name = Column(String(255), nullable=False)
    logo = Column(String(255), nullable=False)
    desc = Column(String, nullable=True)
    system_prompt = Column(String, nullable=True)
    prompt = Column(String, nullable=True)
    guide_word = Column(String, nullable=True)
    guide_question = Column(JSON, nullable=True)
    model_id = Column(Integer, nullable=False)
    temperature = Column(Float, nullable=False)
    max_token = Column(Integer, nullable=False)
    status = Column(Integer, nullable=False)
    user_id = Column(Integer, nullable=False)
    is_delete = Column(Integer, nullable=False)
    knowledges = Column(String, nullable=True)
    tools = Column(String, nullable=True)
    create_time = Column(DateTime, server_default=func.now(), nullable=False)
    update_time = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)
