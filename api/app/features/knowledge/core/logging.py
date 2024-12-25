import logging
import json
from datetime import datetime
from typing import Any, Dict, Optional

class StructuredLogger:
    """Structured logger for knowledge base service"""
    
    def __init__(self, name: str):
        self.logger = logging.getLogger(name)
        self.logger.setLevel(logging.INFO)
    
    def _format_log(
        self,
        level: str,
        message: str,
        event: str,
        user_id: Optional[int] = None,
        **kwargs
    ) -> str:
        """Format log message as JSON"""
        log_data = {
            "timestamp": datetime.utcnow().isoformat(),
            "level": level,
            "event": event,
            "message": message,
            "user_id": user_id,
            **kwargs
        }
        return json.dumps(log_data)
    
    def info(
        self,
        message: str,
        event: str,
        user_id: Optional[int] = None,
        **kwargs
    ) -> None:
        """Log info level message"""
        self.logger.info(
            self._format_log("INFO", message, event, user_id, **kwargs)
        )
    
    def error(
        self,
        message: str,
        event: str,
        user_id: Optional[int] = None,
        error: Optional[Exception] = None,
        **kwargs
    ) -> None:
        """Log error level message"""
        error_details = None
        if error:
            error_details = {
                "type": type(error).__name__,
                "message": str(error)
            }
        self.logger.error(
            self._format_log(
                "ERROR",
                message,
                event,
                user_id,
                error=error_details,
                **kwargs
            )
        )
    
    def warning(
        self,
        message: str,
        event: str,
        user_id: Optional[int] = None,
        **kwargs
    ) -> None:
        """Log warning level message"""
        self.logger.warning(
            self._format_log("WARNING", message, event, user_id, **kwargs)
        )

class AuditLogger(StructuredLogger):
    """Logger for audit events"""
    
    def __init__(self):
        super().__init__("knowledge.audit")
    
    def log_access(
        self,
        user_id: int,
        resource_type: str,
        resource_id: str,
        action: str,
        status: str,
        details: Optional[Dict[str, Any]] = None
    ) -> None:
        """Log access event"""
        self.info(
            f"User {user_id} {action} {resource_type} {resource_id}",
            "access",
            user_id=user_id,
            resource_type=resource_type,
            resource_id=resource_id,
            action=action,
            status=status,
            details=details
        )

class MetricsLogger(StructuredLogger):
    """Logger for metrics and performance"""
    
    def __init__(self):
        super().__init__("knowledge.metrics")
    
    def log_operation(
        self,
        operation: str,
        duration_ms: float,
        success: bool,
        details: Optional[Dict[str, Any]] = None
    ) -> None:
        """Log operation metrics"""
        self.info(
            f"Operation {operation} completed in {duration_ms}ms",
            "operation_metrics",
            operation=operation,
            duration_ms=duration_ms,
            success=success,
            details=details
        )

# Create singleton instances
audit_logger = AuditLogger()
metrics_logger = MetricsLogger()
service_logger = StructuredLogger("knowledge.service")
