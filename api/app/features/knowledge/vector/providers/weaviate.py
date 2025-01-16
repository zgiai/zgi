from typing import List, Dict, Any, Optional
import weaviate
from ..base import VectorDBProvider


class WeaviateProvider(VectorDBProvider):
    """Weaviate vector database provider"""

    def __init__(self, url: str, api_key: str = None, **kwargs):
        """Initialize Weaviate provider
        
        Args:
            url: Weaviate server URL
            api_key: Optional API key
        """
        auth_config = weaviate.auth.AuthApiKey(api_key) if api_key else None
        self.client = weaviate.Client(
            url=url,
            auth_client_secret=auth_config
        )

    async def create_collection(
            self,
            name: str,
            dimension: int,
            **kwargs
    ) -> bool:
        """Create a new Weaviate class"""
        try:
            # class_obj = {
            #     "class": name,
            #     "vectorizer": "none",  # We'll provide vectors directly
            #     "vectorIndexConfig": {
            #         "distance": kwargs.get('metric', 'cosine'),
            #         "dimension": dimension
            #     },
            #     "properties": [
            #         {
            #             "name": "metadata",
            #             "dataType": ["object"],
            #             "description": "Vector metadata",
            #             "nestedProperties": [
            #                 {
            #                     "name": "text",
            #                     "dataType": ["text"],
            #                     "description": "The text content"
            #                 },
            #                 {
            #                     "name": "document_id",
            #                     "dataType": ["int"],
            #                     "description": "The document id"
            #                 },
            #                 {
            #                     "name": "chunk_index",
            #                     "dataType": ["int"],
            #                     "description": "chunk index"
            #                 }
            #             ]
            #         }
            #     ]
            # }
            class_obj = {
                "class": name,
                "vectorizer": "none",  # We'll provide vectors directly
                "vectorIndexConfig": {
                    "distance": kwargs.get('metric', 'cosine'),
                    "dimension": dimension
                },
                "properties": [
                    {
                        "name": "text",
                        "dataType": ["text"],
                        "description": "The text content"
                    },
                    {
                        "name": "document_id",
                        "dataType": ["int"],
                        "description": "The document id"
                    },
                    {
                        "name": "chunk_index",
                        "dataType": ["int"],
                        "description": "chunk index"
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
            with self.client.batch(
                    batch_size=100,
                    dynamic=True,
                    timeout_retries=3
            ) as batch:
                for vector, meta in zip(vectors, metadata):
                    batch.add_data_object(
                        # data_object={"metadata": meta},
                        data_object=meta,
                        class_name=collection_name,
                        vector=vector
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
            # Ensure collection_name is in PascalCase
            collection_name = collection_name.capitalize()
            # Build query
            query = (
                self.client.query
                .get(collection_name, ["document_id chunk_index text"])
                .with_additional(["id", "certainty"])
                .with_near_vector({
                    "vector": query_vector,
                    "certainty": score_threshold if score_threshold else 0.7
                })
                .with_limit(top_k)
            )

            # Add metadata filter if specified
            if metadata_filter:
                # Build the where filter
                where_filter = {
                    "operator": "And",
                    "operands": []
                }
                for key, value in metadata_filter.items():
                    operand = {
                        "path": [key],
                        "operator": "Equal"
                    }
                    if isinstance(value, str):
                        operand["valueString"] = value
                    elif isinstance(value, int):
                        operand["valueInt"] = value
                    elif isinstance(value, bool):
                        operand["valueBoolean"] = value
                    elif isinstance(value, float):
                        operand["valueNumber"] = value
                    else:
                        continue
                    where_filter["operands"].append(operand)
                # Print the constructed where filter for debugging
                print(f"Constructed where_filter: {where_filter}")
                query = query.with_where(where_filter)

            # Execute query
            results = query.do()

            # Format results
            items = results.get('data', {}).get('Get', {}).get(collection_name, [])
            return [{
                'id': item.get('_additional', {}).get('id'),
                'score': item.get('_additional', {}).get('certainty'),
                'document_id': item.get('document_id', {}),
                'chunk_index': item.get('chunk_index', {}),
                'text': item.get('text', {})
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

    async def delete_vectors(
            self,
            collection_name: str,
            metadata_filter: Dict[str, Any] = None) -> bool:
        """Delete vectors from Weaviate class based on metadata filter"""
        try:
            # Ensure collection_name is in PascalCase
            collection_name = collection_name.capitalize()

            where_filter = {}
            if metadata_filter:
                # Build the where filter
                where_filter = {
                    "operator": "And",
                    "operands": []
                }
                for key, value in metadata_filter.items():
                    operand = {
                        "path": [key],
                        "operator": "Equal"
                    }
                    if isinstance(value, str):
                        operand["valueString"] = value
                    elif isinstance(value, int):
                        operand["valueInt"] = value
                    elif isinstance(value, bool):
                        operand["valueBoolean"] = value
                    elif isinstance(value, float):
                        operand["valueNumber"] = value
                    else:
                        continue
                    where_filter["operands"].append(operand)
                # Print the constructed where filter for debugging
                print(f"Constructed where_filter: {where_filter}")

            # Delete objects based on the where filter
            if not where_filter:
                print("No metadata filter provided. Deleting all objects.")
                self.client.schema.delete_class(collection_name)
                return True
            else:
                response = self.client.batch.delete_objects(
                    class_name=collection_name,
                    where=where_filter
                )

                # Print the response for debugging
                print(f"Delete response: {response}")

                if response.get('results', {}).get('successful', 0) > 0:
                    print(f"Successfully deleted {response['results']['successful']} objects.")
                    return True
                else:
                    print("No objects were deleted.")
                    return False
        except Exception as e:
            print(f"Error deleting vectors from Weaviate: {e}")
            return False

    async def get_vectors(
        self,
        collection_name: str,
        metadata_filter: Optional[Dict[str, Any]] = None) -> List[List[float]]:
        """Retrieve vectors and metadata from Weaviate class based on metadata filter"""
        try:
            # Ensure collection_name is in PascalCase
            collection_name = collection_name.capitalize()

            query = (
                self.client.query
                .get(collection_name, ["document_id chunk_index text"])
                .with_additional(["vector"])
            )

            if metadata_filter:
                where_filter = {
                    "operator": "And",
                    "operands": []
                }
                for key, value in metadata_filter.items():
                    operand = {
                        "path": [key],
                        "operator": "Equal"
                    }
                    if isinstance(value, str):
                        operand["valueString"] = value
                    elif isinstance(value, int):
                        operand["valueInt"] = value
                    elif isinstance(value, bool):
                        operand["valueBoolean"] = value
                    elif isinstance(value, float):
                        operand["valueNumber"] = value
                    else:
                        continue
                    where_filter["operands"].append(operand)
                
                if where_filter["operands"]:
                    query = query.with_where(where_filter)

            # 执行查询
            results = query.do()

            items = results.get('data', {}).get('Get', {}).get(collection_name, [])

            vectors = []
            for item in items:
                if '_additional' in item and 'vector' in item['_additional']:
                    vectors.append(item['_additional']['vector'])

            return vectors
        except Exception as e:
            print(f"Error retrieving vectors from Weaviate: {e}")
            return []
