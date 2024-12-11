import os
from typing import List, Dict, Optional, Union
import requests
import logging
from .exceptions import ZGIError, APIError, AuthenticationError, RateLimitError

logger = logging.getLogger(__name__)

class ZGIClient:
    """
    ZGI SDK client for interacting with ZGI AI services.
    
    Args:
        base_url (str): Base URL for the ZGI API
        api_key (str): API key for authentication
        default_model (str, optional): Default model to use for requests
    """
    
    def __init__(
        self,
        base_url: str = "https://api.zgi.ai",
        api_key: str = None,
        default_model: str = "gpt-3.5-turbo"
    ):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key or os.getenv("ZGI_API_KEY")
        if not self.api_key:
            raise AuthenticationError("API key must be provided or set as ZGI_API_KEY environment variable")
        self.default_model = default_model
        self.session = requests.Session()
        self.session.headers.update({
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
            "User-Agent": "zgi-python-sdk/0.1.0"
        })

    def _make_request(
        self,
        method: str,
        endpoint: str,
        params: Optional[Dict] = None,
        json: Optional[Dict] = None,
        files: Optional[Dict] = None,
        timeout: int = 30
    ) -> Dict:
        """Make HTTP request to the API."""
        url = f"{self.base_url}{endpoint}"
        try:
            response = self.session.request(
                method=method,
                url=url,
                params=params,
                json=json,
                files=files,
                timeout=timeout
            )
            response.raise_for_status()
            return response.json()
        except requests.exceptions.HTTPError as e:
            if response.status_code == 401:
                raise AuthenticationError("Invalid API key")
            elif response.status_code == 429:
                raise RateLimitError("Rate limit exceeded")
            else:
                raise APIError(f"HTTP error occurred: {str(e)}")
        except requests.exceptions.RequestException as e:
            raise ZGIError(f"Error making request: {str(e)}")

    def chat(
        self,
        messages: List[Dict[str, str]],
        model: Optional[str] = None,
        temperature: float = 0.7,
        max_tokens: Optional[int] = None,
        stream: bool = False
    ) -> Dict:
        """
        Create a chat completion.
        
        Args:
            messages: List of message dictionaries with 'role' and 'content'
            model: Model to use (defaults to client's default_model)
            temperature: Sampling temperature (0-2)
            max_tokens: Maximum tokens to generate
            stream: Whether to stream the response
        
        Returns:
            API response containing the completion
        """
        json_data = {
            "messages": messages,
            "model": model or self.default_model,
            "temperature": temperature,
            "stream": stream
        }
        if max_tokens is not None:
            json_data["max_tokens"] = max_tokens

        return self._make_request("POST", "/v1/chat/completions", json=json_data)

    def upload_document(
        self,
        kb_id: int,
        file_path: str,
        file_type: Optional[str] = None
    ) -> Dict:
        """
        Upload a document to a knowledge base.
        
        Args:
            kb_id: ID of the knowledge base
            file_path: Path to the file to upload
            file_type: Optional file type override
            
        Returns:
            API response containing the uploaded document details
        """
        with open(file_path, "rb") as f:
            files = {"file": (os.path.basename(file_path), f, file_type)}
            return self._make_request(
                "POST",
                f"/v1/kb/{kb_id}/upload",
                files=files
            )

    def search_documents(
        self,
        kb_id: int,
        query: str,
        top_k: int = 5,
        min_score: float = 0.0
    ) -> Dict:
        """
        Search documents in a knowledge base.
        
        Args:
            kb_id: ID of the knowledge base
            query: Search query
            top_k: Number of results to return
            min_score: Minimum similarity score threshold
            
        Returns:
            API response containing search results
        """
        json_data = {
            "query": query,
            "top_k": top_k,
            "min_score": min_score
        }
        return self._make_request(
            "POST",
            f"/v1/kb/{kb_id}/search",
            json=json_data
        )

    def create_knowledge_base(
        self,
        name: str,
        description: Optional[str] = None,
        visibility: str = "private"
    ) -> Dict:
        """
        Create a new knowledge base.
        
        Args:
            name: Name of the knowledge base
            description: Optional description
            visibility: 'private' or 'public'
            
        Returns:
            API response containing the created knowledge base details
        """
        json_data = {
            "name": name,
            "description": description,
            "visibility": visibility
        }
        return self._make_request("POST", "/v1/kb/create", json=json_data)

    def list_knowledge_bases(
        self,
        page: int = 1,
        page_size: int = 10
    ) -> Dict:
        """
        List available knowledge bases.
        
        Args:
            page: Page number
            page_size: Number of items per page
            
        Returns:
            API response containing list of knowledge bases
        """
        params = {
            "page": page,
            "page_size": page_size
        }
        return self._make_request("GET", "/v1/kb/list", params=params)

    def delete_knowledge_base(self, kb_id: int) -> Dict:
        """
        Delete a knowledge base.
        
        Args:
            kb_id: ID of the knowledge base to delete
            
        Returns:
            API response confirming deletion
        """
        return self._make_request("DELETE", f"/v1/kb/{kb_id}")

    def get_usage_metrics(
        self,
        start_date: str,
        end_date: str
    ) -> Dict:
        """
        Get usage metrics for a date range.
        
        Args:
            start_date: Start date (ISO format)
            end_date: End date (ISO format)
            
        Returns:
            API response containing usage metrics
        """
        json_data = {
            "start_date": start_date,
            "end_date": end_date
        }
        return self._make_request(
            "POST",
            "/v1/analytics/usage-metrics",
            json=json_data
        )
