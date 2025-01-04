from typing import List, Dict, Any, Optional
import pinecone
from ..base import VectorDBProvider

class PineconeProvider(VectorDBProvider):
    """Pinecone vector database provider"""
    
    def __init__(self, api_key: str, environment: str, **kwargs):
        """Initialize Pinecone provider
        
        Args:
            api_key: Pinecone API key
            environment: Pinecone environment
        """
        pinecone.init(api_key=api_key, environment=environment)
        
    async def create_collection(
        self,
        name: str,
        dimension: int,
        **kwargs
    ) -> bool:
        """Create a new Pinecone index"""
        try:
            pinecone.create_index(
                name=name,
                dimension=dimension,
                metric=kwargs.get('metric', 'cosine'),
                pod_type=kwargs.get('pod_type', 'p1')
            )
            return True
        except Exception as e:
            print(f"Error creating Pinecone index: {e}")
            return False
    
    async def delete_collection(self, name: str) -> bool:
        """Delete a Pinecone index"""
        try:
            pinecone.delete_index(name)
            return True
        except Exception as e:
            print(f"Error deleting Pinecone index: {e}")
            return False
    
    async def insert_vectors(
        self,
        collection_name: str,
        vectors: List[List[float]],
        metadata: List[Dict[str, Any]]
    ) -> bool:
        """Insert vectors into Pinecone index"""
        try:
            index = pinecone.Index(collection_name)
            # Create vector IDs
            vector_ids = [f"vec_{i}" for i in range(len(vectors))]
            # Create upsert items
            items = [(vector_ids[i], vectors[i], metadata[i]) 
                    for i in range(len(vectors))]
            # Upsert in batches
            batch_size = 100
            for i in range(0, len(items), batch_size):
                batch = items[i:i + batch_size]
                index.upsert(vectors=batch)
            return True
        except Exception as e:
            print(f"Error inserting vectors into Pinecone: {e}")
            return False
    
    async def search(
        self,
        collection_name: str,
        query_vector: List[float],
        top_k: int = 5,
        score_threshold: Optional[float] = None,
        metadata_filter: Optional[Dict[str, Any]] = None,
        **kwargs
    ) -> List[Dict[str, Any]]:
        """Search similar vectors in Pinecone index"""
        try:
            index = pinecone.Index(collection_name)
            results = index.query(
                vector=query_vector,
                top_k=top_k,
                include_metadata=True,
                filter=metadata_filter
            )
            
            # Filter by score threshold if specified
            matches = results.matches
            if score_threshold is not None:
                matches = [m for m in matches if m.score >= score_threshold]
                
            # Format results
            return [{
                'id': match.id,
                'score': match.score,
                'metadata': match.metadata
            } for match in matches]
        except Exception as e:
            print(f"Error searching vectors in Pinecone: {e}")
            return []
    
    async def get_collection_info(self, name: str) -> Dict[str, Any]:
        """Get Pinecone index information"""
        try:
            index = pinecone.Index(name)
            stats = index.describe_index_stats()
            return {
                'name': name,
                'dimension': stats['dimension'],
                'total_vectors': stats['total_vector_count'],
                'index_fullness': stats['index_fullness']
            }
        except Exception as e:
            print(f"Error getting Pinecone index info: {e}")
            return {}
