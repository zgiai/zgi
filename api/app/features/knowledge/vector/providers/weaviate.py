from typing import List, Dict, Any, Optional
import weaviate
from ..base import VectorDBProvider

class WeaviateProvider(VectorDBProvider):
    """Weaviate vector database provider"""
    
    def __init__(self, url: str, api_key: str = None):
        """Initialize Weaviate provider
        
        Args:
            url: Weaviate server URL
            api_key: Optional API key
        """
        self.client = weaviate.Client(
            url=url,
            auth_client_secret=weaviate.AuthApiKey(api_key) if api_key else None
        )
    
    async def create_collection(
        self,
        name: str,
        dimension: int,
        **kwargs
    ) -> bool:
        """Create a new Weaviate class"""
        try:
            class_obj = {
                "class": name,
                "vectorizer": "none",  # We'll provide vectors directly
                "vectorIndexConfig": {
                    "distance": kwargs.get('metric', 'cosine')
                },
                "properties": [
                    {
                        "name": "metadata",
                        "dataType": ["object"],
                        "description": "Vector metadata"
                    }
                ]
            }
            self.client.schema.create_class(class_obj)
            return True
        except Exception as e:
            print(f"Error creating Weaviate class: {e}")
            return False
    
    async def delete_collection(self, name: str) -> bool:
        """Delete a Weaviate class"""
        try:
            self.client.schema.delete_class(name)
            return True
        except Exception as e:
            print(f"Error deleting Weaviate class: {e}")
            return False
    
    async def insert_vectors(
        self,
        collection_name: str,
        vectors: List[List[float]],
        metadata: List[Dict[str, Any]]
    ) -> bool:
        """Insert vectors into Weaviate class"""
        try:
            # Insert vectors in batches
            batch_size = 100
            with self.client.batch as batch:
                batch.batch_size = batch_size
                for i in range(len(vectors)):
                    batch.add_data_object(
                        data_object={"metadata": metadata[i]},
                        class_name=collection_name,
                        vector=vectors[i]
                    )
            return True
        except Exception as e:
            print(f"Error inserting vectors into Weaviate: {e}")
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
        """Search similar vectors in Weaviate class"""
        try:
            # Build query
            query = (
                self.client.query
                .get(collection_name, ["metadata"])
                .with_near_vector({
                    "vector": query_vector,
                    "certainty": score_threshold if score_threshold else 0.7
                })
                .with_limit(top_k)
            )
            
            # Add metadata filter if specified
            if metadata_filter:
                where_filter = {
                    "path": ["metadata"],
                    "operator": "Equal",
                    "valueObject": metadata_filter
                }
                query = query.with_where(where_filter)
            
            # Execute query
            results = query.do()
            
            # Format results
            items = results.get('data', {}).get('Get', {}).get(collection_name, [])
            return [{
                'id': item.get('_additional', {}).get('id'),
                'score': item.get('_additional', {}).get('certainty'),
                'metadata': item.get('metadata', {})
            } for item in items]
        except Exception as e:
            print(f"Error searching vectors in Weaviate: {e}")
            return []
    
    async def get_collection_info(self, name: str) -> Dict[str, Any]:
        """Get Weaviate class information"""
        try:
            schema = self.client.schema.get(name)
            return {
                'name': name,
                'vectorizer': schema.get('vectorizer'),
                'vector_index_config': schema.get('vectorIndexConfig'),
                'properties': schema.get('properties')
            }
        except Exception as e:
            print(f"Error getting Weaviate class info: {e}")
            return {}
