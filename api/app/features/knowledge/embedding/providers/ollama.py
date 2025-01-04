from typing import List, Dict, Any
import httpx
from ..base import EmbeddingProvider

class OllamaEmbeddingProvider(EmbeddingProvider):
    """Ollama embedding provider"""
    
    def __init__(
        self,
        api_base: str = "http://localhost:11434",
        model: str = "nomic-embed-text:latest",
        **kwargs
    ):
        """Initialize Ollama embedding provider
        
        Args:
            api_base: Base URL for the Ollama API
            model: Model name (default: nomic-embed-text:latest)
            **kwargs: Additional configuration
        """
        self.api_base = api_base
        self.model = model
        
        # Model specific settings
        self._dimensions = {
            "nomic-embed-text:latest": 1536,
        }
        self._max_batch_sizes = {
            "nomic-embed-text:latest": 2048,
        }
        self._max_input_lengths = {
            "nomic-embed-text:latest": 8191,
        }
    
    async def get_embeddings(
        self,
        texts: List[str],
        **kwargs
    ) -> List[List[float]]:
        """Get embeddings using Ollama API"""
        try:
            # Split into batches if needed
            batch_size = self.max_batch_size
            embeddings = []
            
            async with httpx.AsyncClient() as client:
                for i in range(0, len(texts), batch_size):
                    batch = texts[i:i + batch_size]
                    response = await client.post(
                        f"{self.api_base}/v1/embeddings",
                        json={"input": batch, "model": self.model},
                        timeout=30.0
                    )
                    response.raise_for_status()
                    batch_embeddings = response.json().get('embeddings', [])
                    embeddings.extend(batch_embeddings)
            
            return embeddings
        except Exception as e:
            print(f"Error getting Ollama embeddings: {e}")
            return []
    
    def get_dimension(self) -> int:
        """Get embedding dimension for current model"""
        return self._dimensions.get(self.model, 1536)
    
    async def health_check(self) -> bool:
        """Check if Ollama API is accessible"""
        try:
            async with httpx.AsyncClient() as client:
                response = await client.post(
                    f"{self.api_base}/embeddings",
                    json={"input": ["test"], "model": self.model},
                    timeout=10.0
                )
                response.raise_for_status()
                return len(response.json()['embeddings'][0]) == self.get_dimension()
        except Exception as e:
            print(f"Ollama API health check failed: {e}")
            return False
    
    @property
    def max_batch_size(self) -> int:
        """Get maximum batch size for current model"""
        return self._max_batch_sizes.get(self.model, 2048)
    
    @property
    def max_input_length(self) -> int:
        """Get maximum input length for current model"""
        return self._max_input_lengths.get(self.model, 8191)