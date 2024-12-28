from typing import List, Dict, Any
import openai
from ..base import EmbeddingProvider

class OpenAIEmbeddingProvider(EmbeddingProvider):
    """OpenAI embedding provider"""
    
    def __init__(
        self,
        api_key: str,
        model: str = "text-embedding-3-small",
        **kwargs
    ):
        """Initialize OpenAI embedding provider
        
        Args:
            api_key: OpenAI API key
            model: Model name (default: text-embedding-3-small)
            **kwargs: Additional configuration
        """
        self.api_key = api_key
        self.model = model
        openai.api_key = api_key
        
        # Model specific settings
        self._dimensions = {
            "text-embedding-3-small": 1536,
            "text-embedding-3-large": 3072,
            "text-embedding-ada-002": 1536
        }
        self._max_batch_sizes = {
            "text-embedding-3-small": 2048,
            "text-embedding-3-large": 2048,
            "text-embedding-ada-002": 2048
        }
        self._max_input_lengths = {
            "text-embedding-3-small": 8191,
            "text-embedding-3-large": 8191,
            "text-embedding-ada-002": 8191
        }
    
    async def get_embeddings(
        self,
        texts: List[str],
        **kwargs
    ) -> List[List[float]]:
        """Get embeddings using OpenAI API"""
        try:
            # Split into batches if needed
            batch_size = self.max_batch_size
            embeddings = []
            
            for i in range(0, len(texts), batch_size):
                batch = texts[i:i + batch_size]
                response = await openai.Embedding.acreate(
                    input=batch,
                    model=self.model,
                    **kwargs
                )
                batch_embeddings = [data['embedding'] for data in response['data']]
                embeddings.extend(batch_embeddings)
            
            return embeddings
        except Exception as e:
            print(f"Error getting OpenAI embeddings: {e}")
            return []
    
    def get_dimension(self) -> int:
        """Get embedding dimension for current model"""
        return self._dimensions.get(self.model, 1536)
    
    async def health_check(self) -> bool:
        """Check if OpenAI API is accessible"""
        try:
            # Try to get embedding for a simple text
            response = await openai.Embedding.acreate(
                input="test",
                model=self.model
            )
            return len(response['data'][0]['embedding']) == self.get_dimension()
        except Exception as e:
            print(f"OpenAI API health check failed: {e}")
            return False
    
    @property
    def max_batch_size(self) -> int:
        """Get maximum batch size for current model"""
        return self._max_batch_sizes.get(self.model, 2048)
    
    @property
    def max_input_length(self) -> int:
        """Get maximum input length for current model"""
        return self._max_input_lengths.get(self.model, 8191)
