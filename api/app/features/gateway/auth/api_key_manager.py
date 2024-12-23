"""API key management module."""
import os
from typing import Optional, Dict, Any
import logging
from sqlalchemy.orm import Session
from app.core.database import get_db
from app.features.api_keys.models import APIKey, APIKeyStatus
from app.features.usage.models import ResourceUsage
from app.features.usage.schemas import ResourceUsageCreate
from app.features.usage.service import UsageService
from app.features.projects.models import Project

# Configure logging
logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

class APIKeyManager:
    """API key manager for handling user API keys and provider mappings."""
    
    def __init__(self):
        """Initialize the API key manager."""
        self._provider_keys = {
            "anthropic": os.environ.get("ANTHROPIC_API_KEY"),
            "openai": os.environ.get("OPENAI_API_KEY")
        }
        
    async def validate_api_key(
        self,
        api_key: str,
        db: Session
    ) -> Optional[Dict[str, Any]]:
        """Validate a user API key.
        
        Args:
            api_key: User's API key
            db: Database session
            
        Returns:
            User info if key is valid, None otherwise
            
        Example return value:
            {
                "user_id": "user123",
                "project_id": 1,
                "quota": {
                    "daily_limit": 1000,
                    "used_today": 50
                },
                "permissions": ["anthropic", "openai"]
            }
        """
        # Check API key format
        if not api_key.startswith("Bearer "):
            api_key = api_key
        else:
            api_key = api_key[7:]  # Remove "Bearer " prefix
            
        if not api_key.startswith("zgi_"):
            logger.warning(f"Invalid API key format: {api_key}")
            return None
            
        # Query API key from database
        db_key = db.query(APIKey).filter(
            APIKey.key == api_key,
            APIKey.status == APIKeyStatus.ACTIVE
        ).first()
        
        if not db_key:
            logger.warning(f"API key not found or inactive: {api_key}")
            return None
            
        # Get project details
        project = db.query(Project).filter(
            Project.id == db_key.project_id
        ).first()
        
        if not project:
            logger.error(f"Project not found for API key: {api_key}")
            return None
            
        # Get usage statistics
        usage_service = UsageService(db)
        stats = usage_service.get_usage_stats(
            application_id=project.id,
            start_time=None,  # Get all-time stats
            end_time=None
        )
        
        # Return user info
        return {
            "user_id": db_key.created_by,
            "project_id": project.id,
            "quota": {
                "daily_limit": project.quota or 1000000,  # Default to 1M tokens
                "used_today": stats.total_tokens
            },
            "permissions": ["anthropic", "openai"]  # TODO: Get from project settings
        }
        
    def get_provider_key(
        self,
        provider: str,
        user_info: Dict[str, Any]
    ) -> Optional[str]:
        """Get the actual provider API key for a user.
        
        Args:
            provider: Provider name (e.g., "anthropic")
            user_info: User information from validate_api_key
            
        Returns:
            Provider API key if available and allowed
            
        Raises:
            ValueError: If provider is not supported or user is not authorized
        """
        # Check if user has permission to use this provider
        if provider not in user_info["permissions"]:
            logger.warning(f"User {user_info['user_id']} not authorized for provider {provider}")
            raise ValueError(f"Not authorized to use provider: {provider}")
            
        # Check quota
        quota = user_info["quota"]
        if quota["used_today"] >= quota["daily_limit"]:
            logger.warning(f"User {user_info['user_id']} exceeded daily quota")
            raise ValueError("Daily quota exceeded")
            
        # Get provider key
        provider_key = self._provider_keys.get(provider)
        if not provider_key:
            logger.error(f"No API key configured for provider {provider}")
            raise ValueError(f"Provider {provider} is not properly configured")
            
        return provider_key
        
    async def update_usage(
        self,
        user_info: Dict[str, Any],
        tokens_used: int,
        db: Session
    ) -> bool:
        """Update a user's API usage.
        
        Args:
            user_info: User information from validate_api_key
            tokens_used: Number of tokens used in the request
            db: Database session
            
        Returns:
            True if update successful
        """
        try:
            # Create usage record
            usage_service = UsageService(db)
            usage_data = ResourceUsageCreate(
                application_id=user_info["project_id"],
                resource_type="token",
                quantity=float(tokens_used),
                endpoint="/v1/chat/completions"
            )
            usage_service.record_usage(usage_data)
            
            logger.info(f"Recorded {tokens_used} tokens for project {user_info['project_id']}")
            return True
            
        except Exception as e:
            logger.error(f"Error updating usage: {str(e)}")
            return False
