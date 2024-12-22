"""Service for managing user quotas and usage logging"""
from typing import Dict, Any
from sqlalchemy.orm import Session
from datetime import datetime, timedelta
from ..models.user_quota import UserQuota, UsageLog

class QuotaService:
    """Service for managing user quotas and usage tracking"""

    @staticmethod
    async def check_quota(db: Session, api_key: str) -> bool:
        """
        Check if user has sufficient quota.
        
        Args:
            db: Database session
            api_key: User's API key
        
        Returns:
            True if user has sufficient quota, False otherwise
        
        Raises:
            ValueError: If user quota not found
        """
        quota = db.query(UserQuota).filter(UserQuota.api_key == api_key).first()
        if not quota:
            raise ValueError("User quota not found")

        # Check if quota needs to be reset
        if quota.reset_date < datetime.now():
            quota.used_tokens = 0
            quota.reset_date = datetime.now() + timedelta(days=30)
            db.commit()
            return True

        return quota.used_tokens < quota.total_tokens

    @staticmethod
    async def update_usage(
        db: Session,
        api_key: str,
        model: str,
        usage: Dict[str, int],
        cost: float = 0.0
    ) -> None:
        """
        Update user quota and log usage.
        
        Args:
            db: Database session
            api_key: User's API key
            model: Model used for the request
            usage: Token usage information
            cost: Cost of the request
        """
        # Update user quota
        quota = db.query(UserQuota).filter(UserQuota.api_key == api_key).first()
        if quota:
            quota.used_tokens += usage.get("total_tokens", 0)
            db.commit()

        # Log usage
        log = UsageLog(
            api_key=api_key,
            model=model,
            prompt_tokens=usage.get("prompt_tokens", 0),
            completion_tokens=usage.get("completion_tokens", 0),
            total_tokens=usage.get("total_tokens", 0),
            cost=cost
        )
        db.add(log)
        db.commit()
