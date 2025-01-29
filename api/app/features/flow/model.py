from sqlalchemy import Column, Integer, String, DateTime, JSON, ForeignKey
from sqlalchemy.orm import relationship
from app.core.database import Base
from sqlalchemy.sql import func

class Flow(Base):
    __tablename__ = 'flow'

    id = Column(String(32), primary_key=True)
    name = Column(String(255), nullable=False)
    user_id = Column(Integer, index=True)
    description = Column(String(255))
    logo = Column(String(255))
    status = Column(Integer)
    flow_type = Column(Integer)
    is_delete = Column(Integer, nullable=False)
    update_time = Column(DateTime, default=func.now(), onupdate=func.now())
    create_time = Column(DateTime, default=func.now())
    guide_word = Column(String(1000))
    data = Column(JSON)

    versions = relationship('FlowVersion', back_populates='flow')


class FlowVersion(Base):
    __tablename__ = 'flowversion'

    id = Column(Integer, primary_key=True, autoincrement=True)
    flow_id = Column(String(32), ForeignKey('flow.id'), index=True)
    name = Column(String(255), nullable=False)
    data = Column(JSON)
    description = Column(String(255))
    user_id = Column(Integer, index=True)
    flow_type = Column(Integer)
    is_current = Column(Integer)
    is_delete = Column(Integer)
    original_version_id = Column(Integer)
    create_time = Column(DateTime, default=func.now())
    update_time = Column(DateTime, default=func.now(), onupdate=func.now())

    flow = relationship('Flow', back_populates='versions')
