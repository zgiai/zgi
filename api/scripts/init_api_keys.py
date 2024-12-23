"""Initialize API key mappings in the database."""
import os
import sys
import asyncio
from pathlib import Path
from dotenv import load_dotenv
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select

# Add the api directory to the Python path
api_dir = str(Path(__file__).parent.parent)
sys.path.insert(0, api_dir)

from app.core.database import get_db
from app.features.gateway.service.api_key_service import APIKeyService
from app.features.gateway.models.api_key import APIKeyMapping

# Load environment variables
env_path = Path(api_dir) / ".env"
load_dotenv(env_path)

async def init_api_keys():
    """Initialize API key mappings."""
    # Get database session
    async for db in get_db():
        try:
            # Delete existing mappings for test-key
            result = await db.execute(
                select(APIKeyMapping).where(APIKeyMapping.api_key == "test-key")
            )
            existing = result.scalar_one_or_none()
            if existing:
                await db.delete(existing)
                await db.commit()
            
            # Create API key mapping
            provider_keys = {
                "openai": os.getenv("OPENAI_API_KEY"),
                "anthropic": os.getenv("ANTHROPIC_API_KEY"),
                "deepseek": os.getenv("DEEPSEEK_API_KEY")
            }
            
            # Filter out None values
            provider_keys = {k: v for k, v in provider_keys.items() if v}
            
            print("Provider keys to be mapped:")
            for provider, key in provider_keys.items():
                masked_key = f"{key[:8]}...{key[-4:]}" if key else "None"
                print(f"  {provider}: {masked_key}")
            
            # Create default API key mapping
            await APIKeyService.create_mapping(
                db=db,
                api_key="test-key",  # This is the key you'll use in your requests
                provider_keys=provider_keys
            )
            
            print("\nSuccessfully initialized API key mappings")
            print("Use 'test-key' as your API key in requests")
            break
            
        except Exception as e:
            print(f"Error initializing API keys: {e}")
            await db.rollback()
            raise
        finally:
            await db.close()

if __name__ == "__main__":
    asyncio.run(init_api_keys())
