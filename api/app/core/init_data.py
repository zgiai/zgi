import secrets
import string

from app.core.api_key import generate_api_key
from app.core.database import SyncSessionLocal
from app.features import Organization, Project, APIKey

# def generate_api_key() -> str:
#     """Generate a secure API key"""
#     return f"zgi_{secrets.token_urlsafe(32)}"

def init_default_organization_data(user_id):
    with SyncSessionLocal() as session:
        try:
            init_organization = session.query(Organization).filter(Organization.id == 1).one_or_none()
            if not init_organization:
                session.add(Organization(id=1, created_by=user_id, name="Default Organization"))
                session.commit()
            init_project = session.query(Project).filter(Project.id == 1).one_or_none()
            if not init_project:
                session.add(Project(id=1, name="Default Project", created_by=user_id, organization_id=1))
                session.commit()
            init_api_key = session.query(APIKey).filter(APIKey.id == 1).one_or_none()
            if not init_api_key:
                session.add(APIKey(id=1, name="Default API Key", key=generate_api_key(), created_by=user_id, project_id=1))
                session.commit()
        except Exception as e:
            session.rollback()
            raise
        finally:
            session.close()