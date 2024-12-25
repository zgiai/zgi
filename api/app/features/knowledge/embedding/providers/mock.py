from typing import List, Dict, Any
import numpy as np
from ..base import EmbeddingProvider

class MockEmbeddingProvider(EmbeddingProvider):
    """Mock embedding provider for testing"""
    
    def __init__(self, **kwargs):
        """Initialize mock provider"""
        self._dimension = kwargs.get("dimension", 384)  # Default to 384 dimensions
        
    async def get_embeddings(
        self,
        texts: List[str],
        **kwargs
    ) -> List[List[float]]:
        """Get embeddings for a list of texts"""
        # Generate random embeddings with consistent dimensions
        embeddings = []
        for _ in texts:
            # Use a fixed seed for consistent results
            np.random.seed(42)
            embedding = np.random.normal(0, 1, self._dimension).tolist()
            # Normalize the embedding
            norm = np.linalg.norm(embedding)
            embedding = [x / norm for x in embedding]
            embeddings.append(embedding)
        return embeddings
        
    def get_dimension(self) -> int:
        """Get embedding dimension"""
        return self._dimension
        
    async def health_check(self) -> bool:
        """Check if the embedding service is healthy"""
        return True
        
    @property
    def max_batch_size(self) -> int:
        """Maximum batch size for embedding requests"""
        return 100
        
    @property
    def max_input_length(self) -> int:
        """Maximum input length in tokens"""
        return 8192  # Common token limit for many models
