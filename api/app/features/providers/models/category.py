from datetime import datetime
from sqlalchemy import Column, BigInteger, String, DateTime, ForeignKey, Table
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from app.db.base_class import Base

# Association table for many-to-many relationship between categories and models
category_model_association = Table(
    'category_model_association',
    Base.metadata,
    Column('category_id', BigInteger, ForeignKey('model_categories.id')),
    Column('model_id', BigInteger, ForeignKey('model_provider_models.id'))
)

class ModelCategory(Base):
    """Model categories table with hierarchical structure support"""
    __tablename__ = "model_categories"

    id = Column(BigInteger, primary_key=True, autoincrement=True, index=True)
    category_name = Column(String(255), nullable=False, comment="Category name")
    parent_id = Column(BigInteger, ForeignKey("model_categories.id"), nullable=True, comment="Parent category ID or NULL")
    description = Column(String(1000), nullable=True, comment="Optional category description")
    
    # Timestamps
    created_at = Column(DateTime(timezone=True), server_default=func.now(), nullable=False, comment="Creation timestamp")
    updated_at = Column(DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False, comment="Last update timestamp")
    deleted_at = Column(DateTime(timezone=True), nullable=True, comment="Soft delete timestamp")

    # Relationships
    parent = relationship("ModelCategory", remote_side=[id], back_populates="children")
    children = relationship("ModelCategory", back_populates="parent")
    models = relationship(
        "ProviderModel",
        secondary=category_model_association,
        back_populates="categories"
    )

    def mark_deleted(self):
        """Mark the category as deleted"""
        self.deleted_at = func.now()

class ModelModelCategory(Base):
    """Many-to-many relationship between models and categories"""
    __tablename__ = "model_model_categories"

    id = Column(BigInteger, primary_key=True, autoincrement=True, index=True)
    model_id = Column(BigInteger, ForeignKey("model_provider_models.id"), nullable=False, comment="Foreign key to model_provider_models.id")
    category_id = Column(BigInteger, ForeignKey("model_categories.id"), nullable=False, comment="Foreign key to model_categories.id")
    
    created_at = Column(DateTime, nullable=False, default=datetime.utcnow, comment="Creation timestamp")
    deleted_at = Column(DateTime, nullable=True, comment="Soft delete timestamp")
