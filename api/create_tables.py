from app.core.database import Base, engine
from app.features.users.models import User
from app.features.teams.models import Team, TeamInvitation

def create_tables():
    Base.metadata.create_all(bind=engine)

if __name__ == "__main__":
    create_tables()
    print("Tables created successfully!")
