import pytest
import json
import os
from typing import Dict
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
from datetime import datetime

from app.models.rag import Document, QueryLog
from app.features.rag.service import RAGService

class TestRAGIntegration:
    @pytest.fixture(autouse=True)
    def setup(self, client, test_user_token: str, db: Session):
        """Setup test environment"""
        self.client = client
        self.token = test_user_token
        self.headers = {"Authorization": f"Bearer {test_user_token}"}
        self.db = db
        self.test_files = []

    def teardown_method(self):
        """Cleanup test files"""
        for file_path in self.test_files:
            try:
                os.remove(file_path)
            except OSError:
                pass

    def create_test_pdf(self, content: str = "Test content") -> str:
        """Create a test PDF file"""
        file_path = f"test_{datetime.now().timestamp()}.pdf"
        with open(file_path, "wb") as f:
            f.write(b"%PDF-1.5\n")  # PDF header
            f.write(content.encode())
        self.test_files.append(file_path)
        return file_path

    def test_document_upload_and_processing(self):
        """Test document upload and processing workflow"""
        # Create test PDF with specific content
        content = """
        Retrieval-Augmented Generation (RAG) is a technique that combines retrieval and generation
        to create more accurate and contextual responses. It works by first retrieving relevant
        information from a knowledge base, then using that information to generate responses.
        
        Key benefits of RAG include:
        1. Improved accuracy
        2. Up-to-date information
        3. Reduced hallucination
        4. Better context awareness
        """
        pdf_path = self.create_test_pdf(content)

        # Upload document
        with open(pdf_path, "rb") as f:
            response = self.client.post(
                "/v1/rag/upload",
                headers=self.headers,
                files={"file": (os.path.basename(pdf_path), f, "application/pdf")}
            )
        
        assert response.status_code == 200
        doc_data = response.json()
        assert doc_data["status"] in ["processing", "completed"]
        
        # Wait for processing to complete
        doc_id = doc_data["id"]
        for _ in range(10):  # Try for 10 seconds
            response = self.client.get(
                f"/v1/rag/documents/{doc_id}",
                headers=self.headers
            )
            if response.json()["status"] == "completed":
                break
            time.sleep(1)
        
        assert response.json()["status"] == "completed"
        return doc_id

    def test_search_functionality(self, doc_id: int):
        """Test search functionality with different queries"""
        test_queries = [
            {
                "query": "What is RAG?",
                "expected_keywords": ["retrieval", "generation", "technique"]
            },
            {
                "query": "What are the benefits of RAG?",
                "expected_keywords": ["accuracy", "hallucination", "context"]
            }
        ]

        for test in test_queries:
            response = self.client.post(
                "/v1/rag/search",
                headers={**self.headers, "Content-Type": "application/json"},
                json={
                    "query": test["query"],
                    "document_ids": [doc_id],
                    "top_k": 3,
                    "min_score": 0.5
                }
            )
            
            assert response.status_code == 200
            results = response.json()
            
            # Verify search results
            assert len(results["results"]) > 0
            found_keywords = False
            for chunk in results["results"]:
                if any(keyword.lower() in chunk["text"].lower() 
                      for keyword in test["expected_keywords"]):
                    found_keywords = True
                    break
            assert found_keywords, f"Expected keywords not found for query: {test['query']}"

    def test_generate_with_context(self, doc_id: int):
        """Test response generation with context"""
        # First get relevant chunks
        search_response = self.client.post(
            "/v1/rag/search",
            headers={**self.headers, "Content-Type": "application/json"},
            json={
                "query": "What is RAG and its benefits?",
                "document_ids": [doc_id],
                "top_k": 3
            }
        )
        assert search_response.status_code == 200
        chunks = search_response.json()["results"]

        # Generate response using chunks
        generate_response = self.client.post(
            "/v1/rag/generate",
            headers={**self.headers, "Content-Type": "application/json"},
            json={
                "query": "Explain RAG and its main benefits",
                "context_chunks": chunks,
                "temperature": 0.7
            }
        )
        
        assert generate_response.status_code == 200
        result = generate_response.json()
        
        # Verify response content
        assert "response" in result
        assert len(result["response"]) > 0
        assert result["tokens_used"] > 0
        assert "model_used" in result

    def test_combined_query(self, doc_id: int):
        """Test the combined search and generate endpoint"""
        response = self.client.post(
            "/v1/rag/query",
            headers={**self.headers, "Content-Type": "application/json"},
            json={
                "query": "What are the main benefits of using RAG?",
                "document_ids": [doc_id],
                "top_k": 3,
                "temperature": 0.7
            }
        )
        
        assert response.status_code == 200
        result = response.json()
        
        # Verify response structure and content
        assert "response" in result
        assert "context_chunks" in result
        assert len(result["context_chunks"]) > 0
        assert result["tokens_used"] > 0
        
        # Verify response mentions benefits
        response_text = result["response"].lower()
        expected_benefits = ["accuracy", "hallucination", "context"]
        found_benefits = [b for b in expected_benefits if b in response_text]
        assert len(found_benefits) > 0, "Response should mention RAG benefits"

    def test_error_handling(self):
        """Test various error scenarios"""
        # Test invalid file type
        text_path = "test.txt"
        with open(text_path, "w") as f:
            f.write("Not a PDF")
        self.test_files.append(text_path)

        with open(text_path, "rb") as f:
            response = self.client.post(
                "/v1/rag/upload",
                headers=self.headers,
                files={"file": ("test.txt", f, "text/plain")}
            )
        assert response.status_code == 400
        
        # Test non-existent document
        response = self.client.get(
            "/v1/rag/documents/99999",
            headers=self.headers
        )
        assert response.status_code == 404
        
        # Test invalid search parameters
        response = self.client.post(
            "/v1/rag/search",
            headers={**self.headers, "Content-Type": "application/json"},
            json={
                "query": "",  # Empty query
                "top_k": -1  # Invalid top_k
            }
        )
        assert response.status_code == 422

    def test_document_management(self):
        """Test document management features"""
        # Create multiple test documents
        doc_ids = []
        for i in range(3):
            pdf_path = self.create_test_pdf(f"Test content {i}")
            with open(pdf_path, "rb") as f:
                response = self.client.post(
                    "/v1/rag/upload",
                    headers=self.headers,
                    files={"file": (os.path.basename(pdf_path), f, "application/pdf")}
                )
            assert response.status_code == 200
            doc_ids.append(response.json()["id"])

        # List documents
        response = self.client.get(
            "/v1/rag/documents",
            headers=self.headers,
            params={"page": 1, "page_size": 10}
        )
        assert response.status_code == 200
        docs = response.json()
        assert len(docs["items"]) >= len(doc_ids)

        # Delete a document
        response = self.client.delete(
            f"/v1/rag/documents/{doc_ids[0]}",
            headers=self.headers
        )
        assert response.status_code == 200

        # Verify deletion
        response = self.client.get(
            f"/v1/rag/documents/{doc_ids[0]}",
            headers=self.headers
        )
        assert response.status_code == 404

    def run_all_tests(self):
        """Run all tests in sequence"""
        # Upload and process document
        doc_id = self.test_document_upload_and_processing()
        
        # Test search functionality
        self.test_search_functionality(doc_id)
        
        # Test generation
        self.test_generate_with_context(doc_id)
        
        # Test combined query
        self.test_combined_query(doc_id)
        
        # Test error handling
        self.test_error_handling()
        
        # Test document management
        self.test_document_management()

if __name__ == "__main__":
    # This allows running the tests directly with pytest
    pytest.main([__file__])
