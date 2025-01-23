from typing import Dict, Any
from pydantic import Field
from pydantic_settings import BaseSettings


class StorageSettings(BaseSettings):
    """Storage settings"""

    # Default provider
    PROVIDER: str = Field(
        default="local",
        description="Storage provider (local or qiniu)"
    )

    PREFIX: str = Field(
        default="knowledge",
        description="Storage prefix"
    )

    # Local storage settings
    LOCAL_UPLOAD_DIR: str = Field(
        default="/path/to/upload/directory",
        description="Local upload directory"
    )

    # storage settings
    QINIU_ACCESS_KEY: str = Field(
        default="",
        description="Qiniu access key"
    )
    QINIU_SECRET_KEY: str = Field(
        default="",
        description="Qiniu secret key"
    )
    QINIU_BUCKET_NAME: str = Field(
        default="",
        description="Qiniu bucket name"
    )
    QINIU_DOMAIN: str = Field(
        default="",
        description="Qiniu domain"
    )

    @property
    def provider_config(self) -> Dict[str, Any]:
        """Get provider specific configuration

        Returns:
            Dict[str, Any]: Provider configuration
        """
        configs = {
            'local': {
                'upload_dir': self.LOCAL_UPLOAD_DIR
            },
            'qiniu': {
                'access_key': self.QINIU_ACCESS_KEY,
                'secret_key': self.QINIU_SECRET_KEY,
                'bucket_name': self.QINIU_BUCKET_NAME,
                'domain': self.QINIU_DOMAIN,
                'prefix': self.PREFIX
            }
        }
        return configs[self.PROVIDER]

    class Config:
        env_prefix = "STORAGE_"
        case_sensitive = True
