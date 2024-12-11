from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field

class Token(BaseModel):
    """Schema for authentication token response"""
    access_token: str
    token_type: str = "bearer"
