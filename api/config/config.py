import os
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

# OpenAI configuration
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")
OPENAI_BASE_URL = os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1")

# Weaviate configuration
WEAVIATE_URL = os.getenv("WEAVIATE_URL")
WEAVIATE_API_KEY = os.getenv("WEAVIATE_API_KEY")

# Export configuration variables
__all__ = [
    "OPENAI_API_KEY",
    "OPENAI_BASE_URL",
    "WEAVIATE_URL",
    "WEAVIATE_API_KEY"
]
