"""
Database module initialization.
Import commonly used database components here to simplify imports in other modules.
"""

from app.core.database import Base, get_db, handle_db_operation

# Export commonly used components
__all__ = ['Base', 'get_db', 'handle_db_operation']
