from typing import Any, Dict, Callable
from functools import wraps

class APIBase:
    @staticmethod
    def success(
        data: Any = None, 
        msg: str = "success", 
        code: int = 200
    ) -> Dict:
        """统一封装成功响应的格式"""
        return {
            "code": code,
            "msg": msg,
            "data": data
        }
    
    @staticmethod
    def error(
        msg: str = "error",
        data: Any = None,
        code: int = 0
    ) -> Dict:
        """统一封装错误响应的格式"""
        return {
            "code": code,
            "msg": msg,
            "data": data
        }
    
    @staticmethod
    def api_response(msg: str = "success"):
        """装饰器函数，用于统一处理API响应格式"""
        def decorator(func: Callable):
            @wraps(func)
            async def wrapper(*args, **kwargs):
                try:
                    result = await func(*args, **kwargs)
                    
                    # 如果返回值已经是标准格式，直接返回
                    if isinstance(result, dict) and all(k in result for k in ["code", "msg", "data"]):
                        return result
                        
                    return APIBase.success(data=result, msg=msg)
                    
                except Exception as e:
                    # 统一处理异常为错误响应
                    return APIBase.error(msg=str(e))
                    
            return wrapper
        return decorator
