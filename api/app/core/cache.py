from typing import Any, Optional, Union
from datetime import datetime, timedelta
import json
import pickle
from redis import Redis
from fastapi import Depends

from app.core.config import settings


class Cache:
    def __init__(self):
        self.redis = Redis(
            host=settings.REDIS_HOST,
            port=settings.REDIS_PORT,
            password=settings.REDIS_PASSWORD,
            db=settings.REDIS_DB,
            decode_responses=True,
        )
        self.vector_redis = Redis(  # Separate instance for vector cache (binary data)
            host=settings.REDIS_HOST,
            port=settings.REDIS_PORT,
            password=settings.REDIS_PASSWORD,
            db=settings.REDIS_VECTOR_DB,
            decode_responses=False,
        )

    async def get(self, key: str) -> Optional[Any]:
        """Get value from cache."""
        try:
            value = self.redis.get(key)
            if value is None:
                return None
            return json.loads(value)
        except Exception:
            return None

    async def set(
        self,
        key: str,
        value: Any,
        expire: Optional[int] = None,
        nx: bool = False,
    ) -> bool:
        """Set value in cache."""
        try:
            value_str = json.dumps(value)
            if nx:
                return bool(self.redis.setnx(key, value_str))
            self.redis.set(key, value_str, ex=expire)
            return True
        except Exception:
            return False

    async def delete(self, key: str) -> bool:
        """Delete key from cache."""
        try:
            return bool(self.redis.delete(key))
        except Exception:
            return False

    async def exists(self, key: str) -> bool:
        """Check if key exists in cache."""
        try:
            return bool(self.redis.exists(key))
        except Exception:
            return False

    async def increment(self, key: str, amount: int = 1) -> Optional[int]:
        """Increment value by amount."""
        try:
            return self.redis.incrby(key, amount)
        except Exception:
            return None

    async def expire(self, key: str, seconds: int) -> bool:
        """Set key expiration."""
        try:
            return bool(self.redis.expire(key, seconds))
        except Exception:
            return False

    # Vector cache methods
    async def get_vector(self, key: str) -> Optional[Any]:
        """Get vector from cache."""
        try:
            value = self.vector_redis.get(key)
            if value is None:
                return None
            return pickle.loads(value)
        except Exception:
            return None

    async def set_vector(
        self,
        key: str,
        value: Any,
        expire: Optional[int] = None,
    ) -> bool:
        """Set vector in cache."""
        try:
            value_bytes = pickle.dumps(value)
            self.vector_redis.set(key, value_bytes, ex=expire)
            return True
        except Exception:
            return False

    async def delete_vector(self, key: str) -> bool:
        """Delete vector from cache."""
        try:
            return bool(self.vector_redis.delete(key))
        except Exception:
            return False

    # Rate limiting methods
    async def check_rate_limit(
        self,
        key: str,
        limit: int,
        window: int,
    ) -> tuple[bool, int]:
        """Check rate limit for key.
        
        Args:
            key: Rate limit key
            limit: Maximum number of requests
            window: Time window in seconds
            
        Returns:
            Tuple of (is_allowed, remaining_requests)
        """
        try:
            pipeline = self.redis.pipeline()
            now = datetime.utcnow().timestamp()
            window_start = now - window

            # Remove old entries
            pipeline.zremrangebyscore(key, 0, window_start)
            
            # Count requests in current window
            pipeline.zcard(key)
            
            # Add current request
            pipeline.zadd(key, {str(now): now})
            
            # Set expiration
            pipeline.expire(key, window)
            
            _, current_requests, *_ = pipeline.execute()
            
            is_allowed = current_requests <= limit
            remaining = max(0, limit - current_requests)
            
            return is_allowed, remaining
        except Exception:
            return True, limit  # Fail open

    # Cache prefix methods
    async def delete_by_prefix(self, prefix: str) -> int:
        """Delete all keys matching prefix."""
        try:
            keys = self.redis.keys(f"{prefix}*")
            if not keys:
                return 0
            return self.redis.delete(*keys)
        except Exception:
            return 0

    # Cache statistics
    async def get_stats(self) -> dict:
        """Get cache statistics."""
        try:
            info = self.redis.info()
            return {
                "used_memory": info["used_memory"],
                "hits": info["keyspace_hits"],
                "misses": info["keyspace_misses"],
                "keys": info["db0"]["keys"],
                "expires": info["db0"]["expires"],
            }
        except Exception:
            return {}


# Dependency
def get_cache() -> Cache:
    return Cache()
