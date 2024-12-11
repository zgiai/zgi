from sqlalchemy import Column, Integer, String, DateTime
from sqlalchemy.sql import func
from app.core.database import Base

class Settings(Base):
    __tablename__ = "settings"

    id = Column(Integer, primary_key=True, index=True)
    key = Column(String(255), unique=True, nullable=False)
    value = Column(String(255), nullable=False)
    created_at = Column(DateTime, server_default=func.now(), nullable=False)
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now(), nullable=False)

    @classmethod
    def get_setting(cls, db, key: str) -> str:
        setting = db.query(cls).filter(cls.key == key).first()
        return setting.value if setting else None

    @classmethod
    def set_setting(cls, db, key: str, value: str):
        setting = db.query(cls).filter(cls.key == key).first()
        if setting:
            setting.value = value
        else:
            setting = cls(key=key, value=value)
            db.add(setting)
        db.commit()
        return setting
