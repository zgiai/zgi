from datetime import datetime, timedelta
from typing import Any, List, Optional
from pydantic import BaseModel, Field

class CacheConfig(BaseModel):
    enabled: bool = True
    default_ttl: int = Field(default=3600, description="Default TTL in seconds")
    max_size: int = Field(default=1024, description="Max cache size in MB")
    eviction_policy: str = "LRU"

class CacheEntry(BaseModel):
    key: str
    value: Any
    created_at: datetime
    expires_at: datetime
    last_accessed: datetime
    access_count: int
    size_bytes: int

class VectorCache(BaseModel):
    document_id: str
    vector: List[float]
    metadata: dict
    created_at: datetime
    last_used: datetime
    use_count: int

class RateLimitConfig(BaseModel):
    endpoint: str
    limit: int
    window: int = Field(default=60, description="Time window in seconds")
    user_specific: bool = True
    burst_limit: Optional[int] = None

class CacheStats(BaseModel):
    total_entries: int
    total_size_bytes: int
    hit_rate: float
    miss_rate: float
    eviction_count: int
    oldest_entry: datetime
    newest_entry: datetime
