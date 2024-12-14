"""Router package for LLM Gateway."""
from fastapi import APIRouter
from .chat import router as chat_router
from .api_key import router as api_key_router

router = APIRouter()

# API key management routes
router.include_router(api_key_router, tags=["api-keys"])

# Chat completion routes
router.include_router(chat_router, tags=["chat"])
