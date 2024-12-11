import pytest
from datetime import datetime, timedelta
from unittest.mock import Mock, AsyncMock

from app.features.analytics.service import AnalyticsService
from app.features.analytics.schemas import TimeRange, UsageMetrics, SystemMetrics


@pytest.fixture
def mock_db():
    return Mock()


@pytest.fixture
def mock_cache():
    cache = AsyncMock()
    cache.get.return_value = None
    return cache


@pytest.fixture
def analytics_service(mock_db, mock_cache):
    return AnalyticsService(mock_db, mock_cache)


@pytest.fixture
def time_range():
    end = datetime.utcnow()
    start = end - timedelta(days=7)
    return TimeRange(start_date=start, end_date=end)


@pytest.mark.asyncio
async def test_get_usage_metrics(analytics_service, time_range, mock_cache):
    # Setup
    expected_metrics = UsageMetrics(
        api_calls=100,
        total_tokens=5000,
        total_documents=50,
        storage_used=1024.0,
        vector_count=1000,
        timestamp=datetime.utcnow(),
    )
    mock_cache.get.return_value = expected_metrics

    # Test
    metrics = await analytics_service.get_usage_metrics(time_range)

    # Verify
    assert metrics == expected_metrics
    mock_cache.get.assert_called_once()


@pytest.mark.asyncio
async def test_get_system_metrics(analytics_service, mock_cache):
    # Setup
    expected_metrics = SystemMetrics(
        cpu_usage=50.0,
        memory_usage=70.0,
        disk_usage=60.0,
        api_latency=0.1,
        error_rate=0.01,
        timestamp=datetime.utcnow(),
    )
    mock_cache.get.return_value = expected_metrics

    # Test
    metrics = await analytics_service.get_system_metrics()

    # Verify
    assert metrics == expected_metrics
    mock_cache.get.assert_called_once_with("system_metrics")


@pytest.mark.asyncio
async def test_generate_analytics_report(analytics_service, time_range, mock_cache):
    # Setup
    mock_cache.get.return_value = None

    # Test
    report = await analytics_service.generate_analytics_report(time_range)

    # Verify
    assert report.time_range == time_range
    assert isinstance(report.total_users, int)
    assert isinstance(report.active_users, int)
    assert isinstance(report.total_api_calls, int)
    assert isinstance(report.average_response_time, float)
    assert isinstance(report.error_rate, float)
    assert isinstance(report.usage_by_endpoint, dict)
    assert isinstance(report.top_users, list)


@pytest.mark.asyncio
async def test_get_usage_metrics_cache_miss(analytics_service, time_range, mock_cache):
    # Setup
    mock_cache.get.return_value = None

    # Test
    metrics = await analytics_service.get_usage_metrics(time_range)

    # Verify
    assert isinstance(metrics, UsageMetrics)
    mock_cache.get.assert_called_once()
    mock_cache.set.assert_called_once()


@pytest.mark.asyncio
async def test_get_system_metrics_cache_miss(analytics_service, mock_cache):
    # Setup
    mock_cache.get.return_value = None

    # Test
    metrics = await analytics_service.get_system_metrics()

    # Verify
    assert isinstance(metrics, SystemMetrics)
    mock_cache.get.assert_called_once()
    mock_cache.set.assert_called_once()
