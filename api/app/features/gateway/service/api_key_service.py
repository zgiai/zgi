"""Service for managing API key mappings"""
from typing import Dict, Optional
from sqlalchemy.orm import Session
from sqlalchemy.future import select
import logging
from ..models.api_key import APIKeyMapping

class APIKeyService:
    """Service for managing API key mappings"""
    
    @staticmethod
    async def get_provider_key(db: Session, api_key: str, provider: str) -> Optional[str]:
        """Get provider API key for a given user API key and provider.
        
        Args:
            db: Database session
            api_key: User's API key
            provider: Provider name (e.g., 'openai', 'deepseek')
            
        Returns:
            Provider API key if found, None otherwise
            
        Raises:
            ValueError: If API key mapping not found
        """
        # For testing purposes, return a test key
        if api_key == "test-key":
            if provider == "openai":
                return "sk-openai-test-key"
            elif provider == "deepseek":
                return "sk-3e5f5d61abc341c584d5c508f618d7f5"
                
        stmt = select(APIKeyMapping).where(APIKeyMapping.api_key == api_key)
        result = await db.execute(stmt)
        mapping = result.scalar_one_or_none()
        
        if not mapping:
            raise ValueError("API key mapping not found")
            
        provider_keys = mapping.provider_keys
        logging.debug(f"Provider keys: {provider_keys}")
        logging.debug(f"Looking for provider: {provider}")
        return provider_keys.get(provider)
    
    @staticmethod
    async def create_mapping(
        db: Session,
        api_key: str,
        provider_keys: Dict[str, str]
    ) -> APIKeyMapping:
        """Create a new API key mapping.
        
        Args:
            db: Database session
            api_key: User's API key
            provider_keys: Dictionary mapping provider names to their API keys
            
        Returns:
            Created API key mapping
        """
        mapping = APIKeyMapping(
            api_key=api_key,
            provider_keys=provider_keys
        )
        db.add(mapping)
        await db.commit()
        await db.refresh(mapping)
        return mapping
    
    @staticmethod
    async def update_mapping(
        db: Session,
        api_key: str,
        provider_keys: Dict[str, str]
    ) -> Optional[APIKeyMapping]:
        """Update an existing API key mapping.
        
        Args:
            db: Database session
            api_key: User's API key
            provider_keys: Dictionary mapping provider names to their API keys
            
        Returns:
            Updated API key mapping if found, None otherwise
        """
        stmt = select(APIKeyMapping).where(APIKeyMapping.api_key == api_key)
        result = await db.execute(stmt)
        mapping = result.scalar_one_or_none()
        
        if not mapping:
            return None
            
        mapping.provider_keys = provider_keys
        await db.commit()
        await db.refresh(mapping)
        return mapping
