import asyncio
import aiohttp
import json

async def stream_chat():
    """Example of using the streaming chat API"""
    # Your API endpoint
    url = "http://localhost:7001/v1/chat/stream"
    
    # Your authentication token (get this by logging in)
    headers = {
        "Authorization": "Bearer YOUR_ACCESS_TOKEN",
        "Content-Type": "application/json"
    }
    
    # Request data
    data = {
        "model": "gpt-4",
        "messages": [
            {
                "role": "user",
                "content": "Tell me a short story about a robot learning to love."
            }
        ],
        "temperature": 0.7
    }
    
    async with aiohttp.ClientSession() as session:
        async with session.post(url, json=data, headers=headers) as response:
            # Check response status
            if response.status != 200:
                print(f"Error: {response.status}")
                return
            
            # Process streaming response
            async for line in response.content:
                line = line.decode('utf-8')
                if line.startswith('data: '):
                    content = line[6:].strip()  # Remove "data: " prefix
                    
                    # Check for end of stream
                    if content == "[DONE]":
                        break
                    
                    # Print content
                    print(content, end='', flush=True)
            print()  # Final newline

if __name__ == "__main__":
    asyncio.run(stream_chat())
