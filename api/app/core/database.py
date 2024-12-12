from sqlalchemy import create_engine
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.orm import sessionmaker
from sqlalchemy.exc import SQLAlchemyError
from fastapi import HTTPException
import logging
import os
import pymysql

from app.core.config import settings

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Log database connection details
logger.info(f"Connecting to database: {settings.DB_HOST}:{settings.DB_PORT}/{settings.DB_DATABASE}")
logger.info(f"Database URL: {settings.SQLALCHEMY_DATABASE_URL}")

SQLALCHEMY_DATABASE_URL = settings.SQLALCHEMY_DATABASE_URL

engine = create_engine(
    SQLALCHEMY_DATABASE_URL,
    echo=True,
    pool_pre_ping=True,  # Enable connection health checks
    pool_recycle=3600,   # Recycle connections after 1 hour
    pool_size=5,         # Set a reasonable pool size
    max_overflow=10      # Allow up to 10 additional connections
)
SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)

Base = declarative_base()

def get_db():
    db = SessionLocal()
    try:
        yield db
    except SQLAlchemyError as e:
        db.rollback()
        logger.error(f"Database error: {str(e)}")
        raise
    finally:
        db.close()

def handle_db_operation(operation):
    """
    Wrapper for database operations with detailed error handling
    
    Usage:
        result = handle_db_operation(lambda: db.query(User).filter(User.id == user_id).first())
    """
    try:
        return operation()
    except SQLAlchemyError as e:
        logger.error(f"Database operation failed: {str(e)}")
        error_details = {
            "error_type": e.__class__.__name__,
            "error_message": str(e),
            "statement": str(getattr(e, 'statement', '')) if hasattr(e, 'statement') else None,
            "params": getattr(e, 'params', None) if hasattr(e, 'params') else None
        }
        raise HTTPException(
            status_code=500,
            detail={
                "message": "Database operation failed",
                "details": error_details
            }
        )
