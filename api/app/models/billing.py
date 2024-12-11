from sqlalchemy import Column, Integer, String, DateTime, ForeignKey, Float, JSON, Boolean
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func

from app.core.database import Base

class ModelPricing(Base):
    """Model pricing configuration"""
    __tablename__ = "model_pricing"

    id = Column(Integer, primary_key=True, index=True)
    model_name = Column(String(100), nullable=False, unique=True)
    price_per_1k_tokens = Column(Float, nullable=False)  # Price in USD per 1000 tokens
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

class BillingAlert(Base):
    """Billing alert settings"""
    __tablename__ = "billing_alerts"

    id = Column(Integer, primary_key=True, index=True)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=False)
    alert_type = Column(String(50), nullable=False)  # monthly_limit, daily_limit, trend_alert
    threshold_amount = Column(Float, nullable=False)  # Amount in USD
    notification_email = Column(String(255), nullable=False)
    is_active = Column(Boolean, default=True)
    last_triggered_at = Column(DateTime)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    application = relationship("Application", back_populates="billing_alerts")

class BillingRecord(Base):
    """Daily billing records"""
    __tablename__ = "billing_records"

    id = Column(Integer, primary_key=True, index=True)
    application_id = Column(Integer, ForeignKey("applications.id", ondelete="CASCADE"), nullable=False)
    date = Column(DateTime, nullable=False)
    model_costs = Column(JSON, nullable=False)  # {"model_name": cost_in_usd}
    total_cost = Column(Float, nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    # Relationships
    application = relationship("Application", back_populates="billing_records")
