from sqlalchemy import create_engine, text
from sqlalchemy.orm import sessionmaker
import os
from dotenv import load_dotenv
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Load environment variables
load_dotenv()

# Database connection URL
DB_USERNAME = os.getenv('DB_USER', 'root')
DB_PASSWORD = os.getenv('DB_PASSWORD', '')
DB_HOST = os.getenv('DB_HOST', 'localhost')
DB_PORT = os.getenv('DB_PORT', '3306')
DB_DATABASE = os.getenv('DB_DATABASE', 'zgi')
DB_CHARSET = os.getenv('DB_CHARSET', 'utf8mb4')

SQLALCHEMY_DATABASE_URL = f"mysql+pymysql://{DB_USERNAME}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_DATABASE}?charset={DB_CHARSET}"

def test_db_connection():
    try:
        # Create engine
        logger.info(f"Connecting to database: {DB_HOST}:{DB_PORT}/{DB_DATABASE}")
        engine = create_engine(SQLALCHEMY_DATABASE_URL, echo=True)
        
        # Test connection
        with engine.connect() as connection:
            result = connection.execute(text("SELECT VERSION()"))
            version = result.scalar()
            logger.info(f"Successfully connected to MySQL. Version: {version}")
            
        # Create session
        SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)
        db = SessionLocal()
        
        # Test query
        result = db.execute(text("SHOW TABLES"))
        tables = result.fetchall()
        logger.info("Tables in database:")
        for table in tables:
            logger.info(f"- {table[0]}")
            
        return True
    except Exception as e:
        logger.error(f"Error connecting to database: {e}")
        return False

if __name__ == "__main__":
    test_db_connection()
