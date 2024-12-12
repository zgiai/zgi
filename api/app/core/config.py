import os
from pydantic_settings import BaseSettings, SettingsConfigDict
from typing import Optional

# 获取项目根目录的路径
ROOT_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

class Settings(BaseSettings):
    PROJECT_NAME: str = "ZGI AI App"  # 添加项目名称设置
    VERSION: str = "0.1.0"
    API_V1_STR: str = "/v1/console"
    OPENAI_MODEL: str = "gpt-4o"
    WEAVIATE_URL: str
    OPENAI_API_KEY: str
    WEAVIATE_API_KEY: str  # 添加这个字段
    OPENAI_API_BASE: str = "https://api.agicto.cn/v1"  # 添加这个字段
    OPENAI_BASE_URL: str = "https://api.agicto.cn"
    ENVIRONMENT: str = "production"

    # Database Settings
    DB_HOST: str = os.getenv("DB_HOST", "127.0.0.1")
    DB_PORT: int = int(os.getenv("DB_PORT", "3306"))
    DB_USER: str = os.getenv("DB_USER", "root")
    DB_PASSWORD: str = os.getenv("DB_PASSWORD", "")
    DB_DATABASE: str = os.getenv("DB_DATABASE", "zgi")
    
    @property
    def SQLALCHEMY_DATABASE_URL(self) -> str:
        return f"mysql+pymysql://{self.DB_USER}:{self.DB_PASSWORD}@{self.DB_HOST}:{self.DB_PORT}/{self.DB_DATABASE}?charset=utf8mb4&collation=utf8mb4_unicode_ci"

    # Redis
    REDIS_HOST: str = os.getenv("REDIS_HOST", "127.0.0.1")
    REDIS_PORT: int = int(os.getenv("REDIS_PORT", "6379"))
    REDIS_DB: int = int(os.getenv("REDIS_DB", "0"))
    REDIS_PASSWORD: Optional[str] = os.getenv("REDIS_PASSWORD")

    # JWT Settings
    SECRET_KEY: str = os.getenv("SECRET_KEY", "your-secret-key-here")  # 请在生产环境中更改此密钥
    ALGORITHM: str = "HS256"
    ACCESS_TOKEN_EXPIRE_MINUTES: int = 60 * 24 * 7  # 7 days

    # Email
    SMTP_TLS: bool = True
    SMTP_PORT: Optional[int] = None
    SMTP_HOST: Optional[str] = None
    SMTP_USER: Optional[str] = None
    SMTP_PASSWORD: Optional[str] = None
    EMAILS_FROM_EMAIL: Optional[str] = None
    EMAILS_FROM_NAME: Optional[str] = None

    # API
    MAX_API_KEYS_PER_USER: int = 5
    MAX_APPLICATIONS_PER_USER: int = 10

    # Cache
    CACHE_EXPIRE_MINUTES: int = 60 * 24  # 24 hours

    # Logging
    LOG_LEVEL: str = "INFO"
    LOG_FORMAT: str = "%(asctime)s - %(name)s - %(levelname)s - %(message)s"

    # Cors
    CORS_ORIGINS: list = ["*"]
    CORS_METHODS: list = ["*"]
    CORS_HEADERS: list = ["*"]

    # File upload settings
    UPLOAD_DIR: str = "uploads"
    MAX_UPLOAD_SIZE: int = 10 * 1024 * 1024  # 10MB

    # RAG settings
    PINECONE_API_KEY: str
    PINECONE_ENVIRONMENT: str
    PINECONE_INDEX_NAME: str

    # Vector settings
    EMBEDDING_MODEL: str = "all-MiniLM-L6-v2"
    CHUNK_SIZE: int = 1000
    CHUNK_OVERLAP: int = 100

    class Config:
        env_file = os.path.join(ROOT_DIR, ".env.test" if os.getenv("TESTING") else ".env")
        env_file_encoding = "utf-8"
        extra = "allow"  # 允许额外的字段
        case_sensitive = True

settings = Settings()

# 打印 .env 文件的路径（仅用于调试，之后请删除）
print(f"Loading .env from: {os.path.join(ROOT_DIR, '.env' if not os.getenv('TESTING') else '.env.test')}")
