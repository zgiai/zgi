import numpy as np
from typing import List, Dict, Any
import os
import openai

from .weaviate_manager import WeaviateManager
from config import config
from dotenv import load_dotenv
from sklearn.metrics.pairwise import cosine_similarity
import logging
from app.models.document import Document


load_dotenv()

# Add other helper functions if needed

class VectorStore:
    def __init__(self):
        self.vectors = []
        self.metadata = []
        self.weaviate_manager = WeaviateManager()
        self.weaviate_manager.connect()
        self.client = self.weaviate_manager.client

    def add_vector(self, vector: np.ndarray, metadata: Dict[str, Any]):
        self.vectors.append(vector)
        self.metadata.append(metadata)

    async def search_vectors(self, query_vector: np.ndarray, top_k: int = 5) -> List[Dict[str, Any]]:
        """
        搜索与查询向量最相似的向量。

        :param query_vector: 查询向量
        :param top_k: 返回的最相似结果数量
        :return: 包含相似度得分和元数据的结果列表
        """

        # 计算余弦相似度
        similarities = cosine_similarity(query_vector.reshape(1, -1))
        print(similarities,'>>>>>>>similarities')

        # 获取top_k个最相似的索引
        top_indices = np.argsort(similarities)[-top_k:][::-1]

        # 构建结果
        results = []
        for idx in top_indices:
            results.append({
                "score": float(similarities[idx]),
                "content": self.vectors[idx].tolist(),  # 将numpy数组转换为列表
                "metadata": self.metadata[idx]
            })

        return results
    def chunk_document(self, document: Document, chunk_size: int = 1000) -> List[str]:
    # 简单的分块策略，每100个单词一个块
        words = document.content.split()
        return [' '.join(words[i:i+100]) for i in range(0, len(words), 100)]

    async def embed_chunks(self, chunks: List[str]) -> List[List[float]]:
        """
        将文本块转换为向量
        
        Args:
            chunks: 文本块列表
            
        Returns:
            List[List[float]]: 向量列表
        """
        vectors = []
        for chunk in chunks:
            try:
                vector = await self.get_embedding(chunk)
                vectors.append(vector)
            except Exception as e:
                logging.error(f"Error embedding chunk: {str(e)}")
                raise
                
        logging.info(f"Successfully embedded {len(vectors)} chunks")
        return vectors

    def store_vectors(self, filename: str, chunks: List[str], vectors: List[List[float]]) -> None:
        """
        将向量存储到 Weaviate
        
        Args:
            filename: 文件名，用作标识
            chunks: 原始文本块列表
            vectors: 向量列表
        """
        try:
            for i, (chunk, vector) in enumerate(zip(chunks, vectors)):
                data_object = {
                    "content": chunk,  # 存储实际的文本内容，而不是占位符
                    "metadata": {
                        "source": filename,
                        "chunk_index": i
                    }
                }
                # 添加 vector 参数来存储向量数据
                self.weaviate_manager.add_object(
                    class_name="Documents", 
                    data_object=data_object,
                    vector=vector  # 传入·实际的向量数据
                )
            
            logging.info(f"Successfully stored {len(vectors)} vectors for {filename}")
        except Exception as e:
            logging.error(f"Error storing vectors: {str(e)}")
            raise

    async def get_embedding(self, text: str) -> List[float]:
        openai.api_key = config.OPENAI_API_KEY
        response = openai.embeddings.create(
            model="text-embedding-ada-002",
            input=text
        ) 
        embedding = response.data[0].embedding
        
        return embedding

    
    async def query_vector_store(self,query_text: str, top_k: int = 5) -> List[Dict[str, Any]]:
        try:
            # First get the embedding for the query text
            query_vector = await self.get_embedding(query_text)
            
            results = self.weaviate_manager.query_objects(
                class_name="Documents",
                vector=query_vector,  # This will now be a list of floats
                limit=top_k
            )
            return results
        except Exception as e:
            logging.error(f"Error querying vector store: {str(e)}")
            raise
        
    def close(self):
        self.client.close()
# Ensure this line is at the end of the file
__all__ = ['VectorStore']  # Include other functions you want to export

