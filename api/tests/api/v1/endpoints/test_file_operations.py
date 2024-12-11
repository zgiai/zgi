import pytest
from fastapi.testclient import TestClient
from main import app
from unittest.mock import patch, MagicMock

client = TestClient(app)

@pytest.fixture
def mock_file():
    return MagicMock(
        filename="test.txt",
        content_type="text/plain",
        file=MagicMock(read=MagicMock(return_value=b"Test content"))
    )

@patch("app.services.file_service.read_file")
def test_read_file(mock_read_file, mock_file):
    mock_read_file.return_value = {
        "filename": "test.txt",
        "content": "Test content",
        "file_type": "text/plain"
    }

    response = client.post("/api/v1/files/read_file", files={"file": ("test.txt", b"Test content", "text/plain")})

    assert response.status_code == 200
    assert response.json() == {
        "filename": "test.txt",
        "content": "Test content",
        "file_type": "text/plain"
    }

    mock_read_file.assert_called_once()

@patch("app.services.file_service.read_file")
def test_read_file_error(mock_read_file, mock_file):
    mock_read_file.side_effect = Exception("File read error")

    response = client.post("/api/v1/files/read_file", files={"file": ("test.txt", b"Test content", "text/plain")})

    assert response.status_code == 400
    assert response.json() == {"error": "File read error"}

    mock_read_file.assert_called_once()
