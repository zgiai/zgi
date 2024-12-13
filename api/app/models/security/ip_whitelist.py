# Moved to app.features.api_keys.models
# class IPWhitelist(Base):
#     """IP whitelist model"""
#     __tablename__ = "ip_whitelists"

#     id = Column(Integer, primary_key=True, index=True)
#     api_key_id = Column(Integer, ForeignKey("api_keys.id", ondelete="CASCADE"), nullable=False, unique=True)
#     allowed_ips = Column(JSON, nullable=False)
#     is_active = Column(Boolean, default=True)
#     created_at = Column(DateTime, server_default=func.now(), nullable=False)
#     updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

#     # Relationship
#     # api_key = relationship("APIKey", back_populates="ip_whitelist")
