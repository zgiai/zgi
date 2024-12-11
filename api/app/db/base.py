from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.orm import as_declarative, declared_attr

@as_declarative()
class Base:
    @declared_attr
    def __tablename__(cls) -> str:
        return cls.__name__.lower()

    # Generate __tablename__ automatically
    @declared_attr
    def __table_args__(cls) -> dict:
        return {'mysql_engine': 'InnoDB'}