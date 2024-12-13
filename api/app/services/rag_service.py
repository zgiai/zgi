import logging
import traceback
from typing import List, Optional
from app.models.document import Document
from app.utils.vector_store import VectorStore
from openai import OpenAI, OpenAIError, APIConnectionError, AsyncOpenAI
from app.core.config import settings

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class RAGService:
    def __init__(self):
        self.vector_store = VectorStore()
        self.client = AsyncOpenAI(
            api_key=settings.OPENAI_API_KEY,
            base_url=settings.OPENAI_API_BASE
        )
        logger.info("Initialized RAG service with vector store and OpenAI client")

    async def process_document(self, document: Document) -> None:
        """Process a document by chunking and vectorizing it"""
        try:
            chunks = self.chunk_document(document)
            vectors = await self.vector_store.embed_chunks(chunks)
            await self.vector_store.store_vectors(document.filename, chunks, vectors)
            logger.info(f"Successfully processed document: {document.filename}")
        except Exception as e:
            logger.error(f"Error processing document: {str(e)}")
            raise

    def chunk_document(self, document: Document, chunk_size: int = 1000) -> List[str]:
        """Split document into chunks of specified size"""
        return self.vector_store.chunk_document(document, chunk_size)

    async def search_similar_chunks(self, query: str, top_k: int = 5) -> List[dict]:
        """Search for similar chunks using the query"""
        try:
            results = await self.vector_store.query_vector_store(query, top_k)
            logger.info(f"Found {len(results)} similar chunks for query")
            return results
        except Exception as e:
            logger.error(f"Error searching similar chunks: {str(e)}")
            raise

    async def generate_response(self, prompt: str, context: Optional[List[str]] = None) -> str:
        """Generate a response using OpenAI API with optional context"""
        try:
            messages = [{"role": "system", "content": "You are a helpful assistant."}]
            
            if context:
                context_str = "\n".join(context)
                messages.append({
                    "role": "system", 
                    "content": f"Here is some relevant context:\n{context_str}"
                })
            
            messages.append({"role": "user", "content": prompt})
            
            response = await self.client.chat.completions.create(
                model=settings.OPENAI_MODEL,
                messages=messages,
                temperature=0.7,
                max_tokens=1000,
                n=1,
                stop=None,
            )
            return response.choices[0].message.content.strip()
        except APIConnectionError as e:
            logger.error(f"APIConnectionError: {str(e)}")
            logger.error(f"Request details: {e.request.method} {e.request.url}")
            logger.error(f"Request headers: {e.request.headers}")
            raise
        except OpenAIError as e:
            logger.error(f"OpenAIError: {str(e)}")
            raise
        except Exception as e:
            logger.error(f"Unexpected error: {str(e)}")
            raise

    async def close(self):
        """Cleanup resources"""
        try:
            if hasattr(self.client, 'close'):
                await self.client.close()
            logger.info("Successfully closed RAG service")
        except Exception as e:
            logger.error(f"Error closing RAG service: {str(e)}")
            raise

rag_service = RAGService()
