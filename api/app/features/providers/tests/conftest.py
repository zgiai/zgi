import pytest
import pytest_asyncio
import asyncio
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool
from app.db.base import Base
from app.core.config import settings
from app.features.providers.models.provider import ModelProvider
from app.features.providers.models.model import ProviderModel
from app.features.providers.models.category import ModelCategory
from app.features.providers.schemas.provider import ProviderCreate
from app.features.providers.schemas.category import CategoryCreate

# Create async test engine using aiomysql
db_url = settings.SQLALCHEMY_DATABASE_URL.replace(
    'mysql+pymysql://',
    'mysql+aiomysql://'
).replace('?charset=utf8mb4&collation=utf8mb4_unicode_ci', '')

engine = create_async_engine(
    db_url,
    poolclass=StaticPool,
    echo=True,
    connect_args={
        'charset': 'utf8mb4'
    }
)

async_session = sessionmaker(
    engine, class_=AsyncSession, expire_on_commit=False
)

@pytest.fixture(scope="session")
def event_loop():
    """Create an instance of the default event loop for each test case."""
    loop = asyncio.get_event_loop_policy().new_event_loop()
    yield loop
    loop.close()

@pytest_asyncio.fixture(scope="function")
async def db_engine():
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield engine
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)

@pytest_asyncio.fixture(scope="function")
async def db(db_engine):
    async with async_session() as session:
        yield session
        await session.rollback()

@pytest_asyncio.fixture(scope="function")
async def test_provider(db):
    from datetime import datetime
    provider_data = ProviderCreate(
        provider_name="Test Provider",
        enabled=True
    )
    provider = ModelProvider(
        **provider_data.model_dump(),
        created_at=datetime.utcnow(),
        updated_at=datetime.utcnow()
    )
    db.add(provider)
    await db.commit()
    await db.refresh(provider)
    return provider

@pytest_asyncio.fixture(scope="function")
async def test_category(db):
    category_data = CategoryCreate(
        name="Test Category",
        description="Test category description"
    )
    category = ModelCategory(**category_data.model_dump())
    db.add(category)
    await db.commit()
    await db.refresh(category)
    return category

@pytest_asyncio.fixture(scope="function")
async def test_model(db, test_provider):
    model = ProviderModel(
        provider_id=test_provider.id,
        model_name="test-model",
        model_version="1.0",
        description="Test model description",
        type="LLM",
        supports_streaming=True,
        supports_function_calling=True
    )
    db.add(model)
    await db.commit()
    await db.refresh(model)
    return model
