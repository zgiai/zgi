import numpy as np
from typing import List, Dict, Any
import os
import openai
from sklearn.metrics.pairwise import cosine_similarity
import logging
from app.models.document import Document
from config import config
from dotenv import load_dotenv

load_dotenv()

class VectorStore:
    def __init__(self):
        self.vectors = []
        self.metadata = []
        self.openai_api_key = os.getenv("OPENAI_API_KEY")
        if not self.openai_api_key:
            raise ValueError("OPENAI_API_KEY environment variable is required")
        openai.api_key = self.openai_api_key

    async def get_embedding(self, text: str) -> List[float]:
        """Get embedding vector for text using OpenAI API"""
        response = openai.embeddings.create(
            model="text-embedding-ada-002",
            input=text
        )
        return response.data[0].embedding

    async def store_vectors(self, filename: str, chunks: List[str], vectors: List[List[float]]) -> None:
        """Store vectors and their metadata in memory"""
        for i, (chunk, vector) in enumerate(zip(chunks, vectors)):
            self.vectors.append(np.array(vector))
            self.metadata.append({
                "content": chunk,
                "source": filename,
                "chunk_index": i
            })
        logging.info(f"Successfully stored {len(vectors)} vectors for {filename}")

    async def query_vector_store(self, query_text: str, top_k: int = 5) -> List[Dict[str, Any]]:
        """Search for similar vectors using cosine similarity"""
        try:
            query_vector = await self.get_embedding(query_text)
            query_vector = np.array(query_vector).reshape(1, -1)
            
            if not self.vectors:
                return []
                
            vectors_matrix = np.vstack(self.vectors)
            similarities = cosine_similarity(query_vector, vectors_matrix)[0]
            
            top_indices = np.argsort(similarities)[-top_k:][::-1]
            
            results = []
            for idx in top_indices:
                results.append({
                    "content": self.metadata[idx]["content"],
                    "metadata": {
                        "source": self.metadata[idx]["source"],
                        "chunk_index": self.metadata[idx]["chunk_index"]
                    },
                    "score": float(similarities[idx])
                })
            return results
            
        except Exception as e:
            logging.error(f"Error querying vector store: {str(e)}")
            raise

    def chunk_document(self, document: Document, chunk_size: int = 1000) -> List[str]:
        """Split document content into chunks"""
        words = document.content.split()
        return [' '.join(words[i:i+chunk_size]) for i in range(0, len(words), chunk_size)]

    async def embed_chunks(self, chunks: List[str]) -> List[List[float]]:
        """Convert text chunks to vectors"""
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

# Ensure this line is at the end of the file
__all__ = ['VectorStore']  # Include other functions you want to export
