import functools
import asyncio
import logging
from typing import TypeVar, Callable, Any, Optional
from fastapi import HTTPException
from .response import ServiceResponse, NotFoundError, AuthorizationError

T = TypeVar('T')
logger = logging.getLogger(__name__)

def handle_service_errors(func: Optional[Callable] = None, *, wrap_response: bool = True):
    """Decorator to handle service errors and optionally wrap response in ServiceResponse"""
    def decorator(func: Callable[..., Any]) -> Callable[..., Any]:
        @functools.wraps(func)
        async def wrapper(*args, **kwargs):
            try:
                result = await func(*args, **kwargs)
                if wrap_response:
                    if isinstance(result, ServiceResponse):
                        return result
                    return ServiceResponse.ok(data=result)
                return result
            except NotFoundError as e:
                logger.error(f"Service error in {func.__name__}: {str(e)}", exc_info=True)
                raise HTTPException(status_code=404, detail=str(e))
            except AuthorizationError as e:
                logger.error(f"Service error in {func.__name__}: {str(e)}", exc_info=True)
                raise HTTPException(status_code=403, detail=str(e))
            except HTTPException as e:
                logger.error(f"HTTP error in {func.__name__}: {str(e)}", exc_info=True)
                raise e
            except Exception as e:
                logger.error(f"Unexpected error in {func.__name__}: {str(e)}", exc_info=True)
                raise HTTPException(status_code=500, detail=str(e))
        return wrapper

    if func is None:
        return decorator
    return decorator(func)

def retry(
    max_attempts: int = 3,
    delay: float = 1.0,
    backoff: float = 2.0,
    exceptions: tuple = (Exception,)
) -> Callable:
    """Retry decorator with exponential backoff"""
    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        async def wrapper(*args, **kwargs) -> Any:
            last_exception = None
            attempt = 0
            current_delay = delay

            while attempt < max_attempts:
                try:
                    return await func(*args, **kwargs)
                except exceptions as e:
                    attempt += 1
                    last_exception = e
                    
                    if attempt == max_attempts:
                        break
                        
                    logger.warning(
                        f"Attempt {attempt} failed for {func.__name__}: {str(e)}. "
                        f"Retrying in {current_delay} seconds..."
                    )
                    await asyncio.sleep(current_delay)
                    current_delay *= backoff

            logger.error(
                f"All {max_attempts} attempts failed for {func.__name__}. "
                f"Last error: {str(last_exception)}"
            )
            raise last_exception

        return wrapper
    return decorator

def async_cache(
    ttl: int = 3600,
    maxsize: int = 128,
    key_prefix: str = ""
) -> Callable:
    """Simple async cache decorator"""
    cache = {}
    
    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        async def wrapper(*args, **kwargs) -> Any:
            # Create cache key from arguments
            key = f"{key_prefix}:{func.__name__}:{str(args)}:{str(kwargs)}"
            
            # Check if result in cache and not expired
            if key in cache:
                result, timestamp = cache[key]
                if asyncio.get_event_loop().time() - timestamp < ttl:
                    return result
                
            # If not in cache or expired, call function
            result = await func(*args, **kwargs)
            
            # Store in cache with current timestamp
            cache[key] = (result, asyncio.get_event_loop().time())
            
            # Maintain cache size
            if len(cache) > maxsize:
                oldest_key = min(cache.keys(), key=lambda k: cache[k][1])
                del cache[oldest_key]
            
            return result
            
        return wrapper
    return decorator
