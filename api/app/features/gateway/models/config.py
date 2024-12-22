from typing import Dict, Optional
from pydantic import BaseModel


class ModelConfig(BaseModel):
    """Configuration for a model in the registry"""
    provider: str
    model_name: str
    base_url: Optional[str] = None
    context_length: int = 4096
    metadata: Dict = {}

    @classmethod
    def from_dict(cls, data: Dict) -> "ModelConfig":
        """Create a ModelConfig instance from a dictionary"""
        return cls(**data)
