import os
from pydantic_settings import BaseSettings, SettingsConfigDict

# 获取项目根目录的路径
ROOT_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

class Settings(BaseSettings):
    PROJECT_NAME: str = "ZGI AI App"  # 添加项目名称设置
    OPENAI_MODEL: str = "gpt-4o"
    WEAVIATE_URL: str
    OPENAI_API_KEY: str
    WEAVIATE_API_KEY: str  # 添加这个字段
    OPENAI_API_BASE: str = "https://api.agicto.cn/v1"  # 添加这个字段

    # JWT Settings
    SECRET_KEY: str = "your-secret-key-here"  # 请在生产环境中更改此密钥
    ALGORITHM: str = "HS256"
    ACCESS_TOKEN_EXPIRE_MINUTES: int = 30

    # Database Settings
    DB_CONNECTION: str
    DB_HOST: str
    DB_PORT: int
    DB_DATABASE: str
    DB_USERNAME: str
    DB_PASSWORD: str
    DB_CHARSET: str
    DB_COLLATION: str

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

    @property
    def SQLALCHEMY_DATABASE_URL(self) -> str:
        return f"mysql+pymysql://{self.DB_USERNAME}:{self.DB_PASSWORD}@{self.DB_HOST}:{self.DB_PORT}/{self.DB_DATABASE}?charset={self.DB_CHARSET}"

    model_config = SettingsConfigDict(
        env_file=os.path.join(ROOT_DIR, ".env"),
        env_file_encoding='utf-8',
        extra='ignore'  # 忽略额外的字段
    )

settings = Settings()

# 打印 .env 文件的路径（仅用于调试，之后请删除）
print(f"Loading .env from: {os.path.join(ROOT_DIR, '.env')}")
