"""Manual test script for chat API."""
import asyncio
import httpx
import json
import os
from typing import Dict, Any

async def test_chat_completion(
    model: str,
    messages: list,
    api_key: str = None
) -> Dict[str, Any]:
    """Test chat completion API.
    
    Args:
        model: Model name
        messages: List of messages
        api_key: Optional API key
        
    Returns:
        Response data
    """
    url = "http://localhost:8000/v1/chat/completions"
    
    headers = {
        "Content-Type": "application/json"
    }
    
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
    
    data = {
        "model": model,
        "messages": messages
    }
    
    async with httpx.AsyncClient() as client:
        try:
            response = await client.post(url, json=data, headers=headers)
            response.raise_for_status()
            return response.json()
        except httpx.HTTPError as e:
            print(f"Request failed: {e}")
            return None
        except Exception as e:
            print(f"An error occurred: {str(e)}")
            return None

async def main():
    """Run test cases."""
    # Test messages
    messages = [
        {"role": "user", "content": "What is the capital of France?"}
    ]
    
    # Test with OpenAI
    print("\nTesting OpenAI GPT-3.5...")
    try:
        result = await test_chat_completion(
            model="gpt-3.5-turbo",
            messages=messages,
            api_key="your-openai-key"  # Replace with your API key
        )
        if result:
            print(json.dumps(result, indent=2))
    except Exception as e:
        print(f"OpenAI test failed: {str(e)}")
    
    # Test with Anthropic
    print("\nTesting Anthropic Claude...")
    try:
        result = await test_chat_completion(
            model="claude-3-sonnet-20240229",
            messages=messages,
            api_key="your-anthropic-key"  # Replace with your API key
        )
        if result:
            print(json.dumps(result, indent=2))
    except Exception as e:
        print(f"Anthropic test failed: {str(e)}")
    
    # Test with DeepSeek
    print("\nTesting DeepSeek...")
    try:
        result = await test_chat_completion(
            model="deepseek-chat",
            messages=messages,
            api_key="your-deepseek-key"  # Replace with your API key
        )
        if result:
            print(json.dumps(result, indent=2))
    except Exception as e:
        print(f"DeepSeek test failed: {str(e)}")
    
    # Test with invalid auth header
    print("\nTesting invalid auth header...")
    try:
        url = "http://localhost:8000/v1/chat/completions"
        headers = {
            "Content-Type": "application/json",
            "Authorization": "Invalid-Auth-Header"
        }
        data = {
            "model": "gpt-3.5-turbo",
            "messages": messages
        }
        async with httpx.AsyncClient() as client:
            response = await client.post(url, json=data, headers=headers)
            response.raise_for_status()
            print(json.dumps(response.json(), indent=2))
    except httpx.HTTPError as e:
        print(f"Invalid auth test failed: {e}")
    except Exception as e:
        print(f"An error occurred: {str(e)}")

if __name__ == "__main__":
    asyncio.run(main())
