import { NextResponse } from 'next/server'

export const runtime = 'edge' // 使用 Edge Runtime 以支持流式响应

export async function POST(request: Request) {
  try {
    const body = await request.json()
    
    const response = await fetch('https://api.zgi.ai/v1/chat/completions', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(body)
    })

    // 确保我们获得了流式响应
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }
    
    // 创建一个 TransformStream 来处理响应
    const encoder = new TextEncoder()
    const decoder = new TextDecoder()
    
    const stream = new TransformStream({
      async transform(chunk, controller) {
        const text = decoder.decode(chunk)
        // 将原始响应直接传递给客户端
        controller.enqueue(encoder.encode(text))
      },
    })

    return new Response(response.body?.pipeThrough(stream), {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
      },
    })

  } catch (error) {
    console.error('Error:', error)
    return NextResponse.json(
      { error: 'Internal Server Error' },
      { status: 500 }
    )
  }
} 