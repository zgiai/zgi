import json
import logging
from datetime import datetime
from typing import Optional, Dict, Any
from fastapi import Request, Response
from sqlalchemy.orm import Session
from starlette.middleware.base import BaseHTTPMiddleware
import uuid

from app.models.security import APILog
from app.core.database import get_db

logger = logging.getLogger(__name__)

class APILogger:
    """Handle API call logging"""
    
    def __init__(self, db: Session):
        self.db = db

    def log_api_call(
        self,
        request_id: str,
        method: str,
        path: str,
        client_ip: str,
        request_data: Optional[Dict[str, Any]] = None,
        response_data: Optional[Dict[str, Any]] = None,
        status_code: Optional[int] = None,
        api_key_id: Optional[str] = None,
        error_message: Optional[str] = None,
        duration_ms: Optional[float] = None
    ) -> APILog:
        """Log an API call to the database"""
        log_entry = APILog(
            request_id=request_id,
            timestamp=datetime.utcnow(),
            method=method,
            path=path,
            client_ip=client_ip,
            request_data=json.dumps(request_data) if request_data else None,
            response_data=json.dumps(response_data) if response_data else None,
            status_code=status_code,
            api_key_id=api_key_id,
            error_message=error_message,
            duration_ms=duration_ms
        )
        
        try:
            self.db.add(log_entry)
            self.db.commit()
            self.db.refresh(log_entry)
            return log_entry
        except Exception as e:
            logger.error(f"Failed to log API call: {str(e)}")
            self.db.rollback()
            raise

class APILoggingMiddleware(BaseHTTPMiddleware):
    """Middleware to log all API calls"""
    
    async def dispatch(self, request: Request, call_next) -> Response:
        # Generate unique request ID
        request_id = str(uuid.uuid4())
        
        # Get start time
        start_time = datetime.utcnow()
        
        # Extract request data
        client_ip = request.client.host
        method = request.method
        path = request.url.path
        api_key = request.headers.get("X-API-Key")
        
        # Try to get request body
        try:
            request_data = await request.json()
        except:
            request_data = None

        # Get database session
        db = next(get_db())
        logger_instance = APILogger(db)
        
        try:
            # Call next middleware/endpoint
            response = await call_next(request)
            
            # Calculate duration
            duration = (datetime.utcnow() - start_time).total_seconds() * 1000
            
            # Try to get response body
            response_body = b""
            async for chunk in response.body_iterator:
                response_body += chunk
            
            try:
                response_data = json.loads(response_body)
            except:
                response_data = None
            
            # Log the successful request
            logger_instance.log_api_call(
                request_id=request_id,
                method=method,
                path=path,
                client_ip=client_ip,
                request_data=request_data,
                response_data=response_data,
                status_code=response.status_code,
                api_key_id=api_key,
                duration_ms=duration
            )
            
            # Reconstruct response
            return Response(
                content=response_body,
                status_code=response.status_code,
                headers=dict(response.headers),
                media_type=response.media_type
            )
            
        except Exception as e:
            # Log the failed request
            duration = (datetime.utcnow() - start_time).total_seconds() * 1000
            logger_instance.log_api_call(
                request_id=request_id,
                method=method,
                path=path,
                client_ip=client_ip,
                request_data=request_data,
                status_code=500,
                api_key_id=api_key,
                error_message=str(e),
                duration_ms=duration
            )
            raise
        finally:
            db.close()

class APILogRetriever:
    """Retrieve and analyze API logs"""
    
    def __init__(self, db: Session):
        self.db = db

    def get_logs(
        self,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        api_key_id: Optional[str] = None,
        path: Optional[str] = None,
        status_code: Optional[int] = None,
        limit: int = 100
    ) -> list[APILog]:
        """Retrieve API logs with filters"""
        query = self.db.query(APILog)
        
        if start_time:
            query = query.filter(APILog.timestamp >= start_time)
        if end_time:
            query = query.filter(APILog.timestamp <= end_time)
        if api_key_id:
            query = query.filter(APILog.api_key_id == api_key_id)
        if path:
            query = query.filter(APILog.path.like(f"%{path}%"))
        if status_code:
            query = query.filter(APILog.status_code == status_code)
        
        return query.order_by(APILog.timestamp.desc()).limit(limit).all()

    def get_error_logs(
        self,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        limit: int = 100
    ) -> list[APILog]:
        """Retrieve error logs"""
        query = self.db.query(APILog).filter(APILog.status_code >= 400)
        
        if start_time:
            query = query.filter(APILog.timestamp >= start_time)
        if end_time:
            query = query.filter(APILog.timestamp <= end_time)
        
        return query.order_by(APILog.timestamp.desc()).limit(limit).all()

    def get_performance_stats(
        self,
        path: str,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None
    ) -> Dict[str, float]:
        """Get performance statistics for an endpoint"""
        query = self.db.query(
            func.avg(APILog.duration_ms).label('avg_duration'),
            func.min(APILog.duration_ms).label('min_duration'),
            func.max(APILog.duration_ms).label('max_duration')
        ).filter(APILog.path == path)
        
        if start_time:
            query = query.filter(APILog.timestamp >= start_time)
        if end_time:
            query = query.filter(APILog.timestamp <= end_time)
        
        result = query.first()
        return {
            'average_duration_ms': float(result.avg_duration or 0),
            'min_duration_ms': float(result.min_duration or 0),
            'max_duration_ms': float(result.max_duration or 0)
        }
