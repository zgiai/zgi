from typing import Any, Dict, List, Optional, Union
from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import AnyHttpUrl, EmailStr, HttpUrl, validator
import os
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

class Settings(BaseSettings):
    PROJECT_NAME: str = "ZGI API"
    VERSION: str = "1.0.0"
    API_V1_STR: str = "/api/v1"
    
    # Server settings
    HOST: str = os.getenv("HOST", "0.0.0.0")
    PORT: int = int(os.getenv("PORT", "7001"))
    
    # Security settings
    SECRET_KEY: str = os.getenv("SECRET_KEY", "test-secret-key-for-development-only")
    ALGORITHM: str = os.getenv("ALGORITHM", "HS256")  # Default to HS256 for JWT encoding
    ACCESS_TOKEN_EXPIRE_MINUTES: int = int(os.getenv("ACCESS_TOKEN_EXPIRE_MINUTES", "30"))
    
    # BACKEND_CORS_ORIGINS is a comma-separated list of origins
    BACKEND_CORS_ORIGINS: List[AnyHttpUrl] = []

    @validator("BACKEND_CORS_ORIGINS", pre=True)
    def assemble_cors_origins(cls, v: Union[str, List[str]]) -> Union[List[str], str]:
        if isinstance(v, str) and not v.startswith("["):
            return [i.strip() for i in v.split(",")]
        elif isinstance(v, (list, str)):
            return v
        raise ValueError(v)

    # Database settings
    MYSQL_SERVER: str = os.getenv("MYSQL_SERVER", "127.0.0.1")
    MYSQL_USER: str = os.getenv("MYSQL_USER", "root")
    MYSQL_PASSWORD: str = os.getenv("MYSQL_PASSWORD", "")
    MYSQL_DB: str = os.getenv("MYSQL_DB", "zgi")
    MYSQL_PORT: str = os.getenv("MYSQL_PORT", "3306")
    SQL_ECHO: bool = os.getenv("SQL_ECHO", "False").lower() == "true"

    # Database connection
    DB_CONNECTION: str = os.getenv("DB_CONNECTION", "mysql")
    DB_HOST: str = os.getenv("DB_HOST", "127.0.0.1")
    DB_PORT: str = os.getenv("DB_PORT", "3306")
    DB_DATABASE: str = os.getenv("DB_DATABASE", "zgi")
    DB_USERNAME: str = os.getenv("DB_USERNAME", "root")
    DB_PASSWORD: str = os.getenv("DB_PASSWORD", "")
    DB_CHARSET: str = os.getenv("DB_CHARSET", "utf8mb4")
    DB_COLLATION: str = os.getenv("DB_COLLATION", "utf8mb4_unicode_ci")

    # OpenAI settings
    OPENAI_API_KEY: str = os.getenv("OPENAI_API_KEY", "")
    OPENAI_API_BASE: str = os.getenv("OPENAI_API_BASE", "https://api.openai.com/v1")
    OPENAI_MODEL: str = os.getenv("OPENAI_MODEL", "gpt-4")
    OPENAI_BASE_URL: str = os.getenv("OPENAI_BASE_URL", "https://api.openai.com")

    # Other LLM providers
    GROQ_API_KEY: str = os.getenv("GROQ_API_KEY", "")
    ANTHROPIC_API_KEY: str = os.getenv("ANTHROPIC_API_KEY", "")
    ANTHROPIC_API_BASE: str = os.getenv("ANTHROPIC_API_BASE", "")
    DEEPSEEK_API_KEY: str = os.getenv("DEEPSEEK_API_KEY", "")
    DEEPSEEK_API_BASE: str = os.getenv("DEEPSEEK_API_BASE", "")

    # Production API keys
    PROD_OPENAI_API_KEY: str = os.getenv("PROD_OPENAI_API_KEY", "")
    PROD_ANTHROPIC_API_KEY: str = os.getenv("PROD_ANTHROPIC_API_KEY", "")
    PROD_DEEPSEEK_API_KEY: str = os.getenv("PROD_DEEPSEEK_API_KEY", "")

    # Environment
    ENVIRONMENT: str = os.getenv("ENVIRONMENT", "development")

    # Vector database settings
    VECTOR_DB_PROVIDER: str = os.getenv("VECTOR_DB_PROVIDER", "pinecone")
    PINECONE_API_KEY: str = os.getenv("PINECONE_API_KEY", "")
    PINECONE_ENVIRONMENT: str = os.getenv("PINECONE_ENVIRONMENT", "")
    PINECONE_INDEX_NAME: str = os.getenv("PINECONE_INDEX_NAME", "zgi")
    
    WEAVIATE_URL: str = os.getenv("WEAVIATE_URL", "")
    WEAVIATE_API_KEY: str = os.getenv("WEAVIATE_API_KEY", "")

    # Embedding settings
    EMBEDDING_PROVIDER: str = os.getenv("EMBEDDING_PROVIDER", "openai")
    EMBEDDING_OPENAI_API_KEY: str = os.getenv("EMBEDDING_OPENAI_API_KEY", "")
    EMBEDDING_OPENAI_MODEL: str = os.getenv("EMBEDDING_OPENAI_MODEL", "text-embedding-3-small")

    # File upload settings
    UPLOAD_DIR: str = "uploads"
    MAX_UPLOAD_SIZE: int = 10 * 1024 * 1024  # 10MB
    ALLOWED_EXTENSIONS: List[str] = ["txt", "pdf", "doc", "docx"]

    @property
    def SQLALCHEMY_DATABASE_URL(self) -> str:
        return f"mysql+aiomysql://{self.MYSQL_USER}:{self.MYSQL_PASSWORD}@{self.MYSQL_SERVER}:{self.MYSQL_PORT}/{self.MYSQL_DB}?charset=utf8mb4"

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=True,
        extra="allow"
    )

settings = Settings()
