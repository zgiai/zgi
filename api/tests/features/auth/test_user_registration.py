import os
import sys
from pathlib import Path

# Get the absolute path to the project root
project_root = Path(__file__).resolve().parent.parent.parent.parent

# Add project root to Python path
sys.path.insert(0, str(project_root))

from sqlalchemy import create_engine, text
from sqlalchemy.orm import sessionmaker
from dotenv import load_dotenv
import logging

# Import app modules
from app.core.database import Base
from app.features.users.models import User  # Updated import path
from app.core.security import get_password_hash

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Load environment variables
load_dotenv(os.path.join(Path(__file__).parent, '.env.test'))

# Database connection URL
DB_USERNAME = os.getenv('DB_USERNAME', 'root')
DB_PASSWORD = os.getenv('DB_PASSWORD', '')
DB_HOST = os.getenv('DB_HOST', 'localhost')
DB_PORT = os.getenv('DB_PORT', '3306')
DB_DATABASE = os.getenv('DB_DATABASE', 'zgi')
DB_CHARSET = os.getenv('DB_CHARSET', 'utf8mb4')

SQLALCHEMY_DATABASE_URL = f"mysql+pymysql://{DB_USERNAME}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_DATABASE}?charset={DB_CHARSET}"

def test_user_registration():
    try:
        # Create engine
        logger.info(f"Connecting to database: {DB_HOST}:{DB_PORT}/{DB_DATABASE}")
        engine = create_engine(SQLALCHEMY_DATABASE_URL, echo=True)
        
        # Create tables if they don't exist
        Base.metadata.create_all(bind=engine)
        
        # Create session
        SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)
        db = SessionLocal()
        
        try:
            # Create test user
            email = "test@example.com"
            username = "testuser"
            password = "Test123!@#"
            
            # Check if user already exists
            existing_user = db.query(User).filter(
                (User.email == email) | (User.username == username)
            ).first()
            
            if existing_user:
                logger.info("Test user already exists, deleting...")
                db.delete(existing_user)
                db.commit()
            
            # Create new user
            hashed_password = get_password_hash(password)
            user = User(
                email=email,
                username=username,
                hashed_password=hashed_password,
                is_active=True,
                is_superuser=False
            )
            
            logger.info(f"Creating user: {email}")
            db.add(user)
            db.commit()
            db.refresh(user)
            
            logger.info(f"Successfully created user: {user.email}")
            return True
            
        except Exception as e:
            logger.error(f"Error creating user: {e}")
            db.rollback()
            return False
            
        finally:
            db.close()
            
    except Exception as e:
        logger.error(f"Error connecting to database: {e}")
        return False

if __name__ == "__main__":
    test_user_registration()
