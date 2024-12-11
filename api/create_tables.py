from app.core.database import Base, engine
from app.models import User, Team, TeamInvitation

def create_tables():
    try:
        Base.metadata.create_all(bind=engine)
        print("Tables created successfully!")
    except Exception as e:
        print(f"Error creating tables: {e}")

if __name__ == "__main__":
    create_tables()
