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

    model_config = SettingsConfigDict(
        env_file=os.path.join(ROOT_DIR, ".env"),
        env_file_encoding='utf-8',
        extra='ignore'  # 忽略额外的字段
    )

settings = Settings()

# 打印 .env 文件的路径（仅用于调试，之后请删除）
print(f"Loading .env from: {os.path.join(ROOT_DIR, '.env')}")
