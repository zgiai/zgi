from abc import ABC, abstractmethod
from typing import List, Dict, Any

class EmbeddingProvider(ABC):
    """Base class for embedding providers"""
    
    @abstractmethod
    async def get_embeddings(
        self,
        texts: List[str],
        **kwargs
    ) -> List[List[float]]:
        """Get embeddings for a list of texts
        
        Args:
            texts: List of texts to get embeddings for
            **kwargs: Additional provider-specific parameters
            
        Returns:
            List[List[float]]: List of embeddings
        """
        pass
    
    @abstractmethod
    def get_dimension(self) -> int:
        """Get embedding dimension
        
        Returns:
            int: Embedding dimension
        """
        pass
    
    @abstractmethod
    async def health_check(self) -> bool:
        """Check if the embedding service is healthy
        
        Returns:
            bool: True if healthy, False otherwise
        """
        pass
    
    @property
    @abstractmethod
    def max_batch_size(self) -> int:
        """Maximum batch size for embedding requests
        
        Returns:
            int: Maximum batch size
        """
        pass
    
    @property
    @abstractmethod
    def max_input_length(self) -> int:
        """Maximum input length in tokens
        
        Returns:
            int: Maximum input length
        """
        pass
