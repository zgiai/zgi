import unittest
from unittest.mock import patch, MagicMock
import os
import tempfile

from zgi import ZGIClient
from zgi.exceptions import (
    AuthenticationError,
    APIError,
    RateLimitError,
)


class TestZGIClient(unittest.TestCase):
    def setUp(self):
        self.api_key = "test-api-key"
        self.base_url = "https://api.zgi.ai"
        self.client = ZGIClient(
            base_url=self.base_url,
            api_key=self.api_key,
            default_model="gpt-3.5-turbo"
        )

    def test_initialization(self):
        """Test client initialization."""
        self.assertEqual(self.client.base_url, self.base_url)
        self.assertEqual(self.client.api_key, self.api_key)
        self.assertEqual(self.client.default_model, "gpt-3.5-turbo")

    def test_initialization_without_api_key(self):
        """Test initialization without API key."""
        with self.assertRaises(AuthenticationError):
            ZGIClient(base_url=self.base_url)

    @patch('requests.Session.request')
    def test_chat_completion(self, mock_request):
        """Test chat completion endpoint."""
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "choices": [{"message": {"content": "Hello!"}}]
        }
        mock_response.status_code = 200
        mock_request.return_value = mock_response

        messages = [{"role": "user", "content": "Hi"}]
        response = self.client.chat(messages)

        mock_request.assert_called_once()
        self.assertEqual(response["choices"][0]["message"]["content"], "Hello!")

    @patch('requests.Session.request')
    def test_rate_limit_error(self, mock_request):
        """Test rate limit error handling."""
        mock_response = MagicMock()
        mock_response.status_code = 429
        mock_response.raise_for_status.side_effect = Exception("Rate limit exceeded")
        mock_request.return_value = mock_response

        with self.assertRaises(RateLimitError):
            self.client.chat([{"role": "user", "content": "Hi"}])

    @patch('requests.Session.request')
    def test_authentication_error(self, mock_request):
        """Test authentication error handling."""
        mock_response = MagicMock()
        mock_response.status_code = 401
        mock_response.raise_for_status.side_effect = Exception("Invalid API key")
        mock_request.return_value = mock_response

        with self.assertRaises(AuthenticationError):
            self.client.chat([{"role": "user", "content": "Hi"}])

    def test_upload_document(self):
        """Test document upload."""
        with tempfile.NamedTemporaryFile(suffix=".txt") as tmp_file:
            tmp_file.write(b"Test content")
            tmp_file.flush()

            with patch('requests.Session.request') as mock_request:
                mock_response = MagicMock()
                mock_response.json.return_value = {"id": "doc123"}
                mock_response.status_code = 200
                mock_request.return_value = mock_response

                response = self.client.upload_document(1, tmp_file.name)
                
                mock_request.assert_called_once()
                self.assertEqual(response["id"], "doc123")

    @patch('requests.Session.request')
    def test_search_documents(self, mock_request):
        """Test document search."""
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "results": [{"id": "doc123", "score": 0.9}]
        }
        mock_response.status_code = 200
        mock_request.return_value = mock_response

        response = self.client.search_documents(1, "test query")

        mock_request.assert_called_once()
        self.assertEqual(len(response["results"]), 1)
        self.assertEqual(response["results"][0]["id"], "doc123")

    @patch('requests.Session.request')
    def test_create_knowledge_base(self, mock_request):
        """Test knowledge base creation."""
        mock_response = MagicMock()
        mock_response.json.return_value = {"id": "kb123", "name": "Test KB"}
        mock_response.status_code = 200
        mock_request.return_value = mock_response

        response = self.client.create_knowledge_base("Test KB")

        mock_request.assert_called_once()
        self.assertEqual(response["id"], "kb123")
        self.assertEqual(response["name"], "Test KB")

    @patch('requests.Session.request')
    def test_list_knowledge_bases(self, mock_request):
        """Test listing knowledge bases."""
        mock_response = MagicMock()
        mock_response.json.return_value = {
            "items": [{"id": "kb123", "name": "Test KB"}],
            "total": 1
        }
        mock_response.status_code = 200
        mock_request.return_value = mock_response

        response = self.client.list_knowledge_bases()

        mock_request.assert_called_once()
        self.assertEqual(len(response["items"]), 1)
        self.assertEqual(response["total"], 1)


if __name__ == '__main__':
    unittest.main()
