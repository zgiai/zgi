import { NextResponse } from "next/server";

export const runtime = "edge";

export async function POST(request: Request) {
	try {
		const body = await request.json();

		const response = await fetch("https://api.zgi.ai/v1/chat/completions", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(body),
		});

		// Make sure we get a streaming response
		if (!response.ok) {
			throw new Error(`HTTP error! status: ${response.status}`);
		}

		// Create a TransformStream to handle the response
		const encoder = new TextEncoder();
		const decoder = new TextDecoder();

		const stream = new TransformStream({
			async transform(chunk, controller) {
				const text = decoder.decode(chunk);
				// 将原始响应直接传递给客户端
				controller.enqueue(encoder.encode(text));
			},
		});

		return new Response(response.body?.pipeThrough(stream), {
			headers: {
				"Content-Type": "text/event-stream",
				"Cache-Control": "no-cache",
				Connection: "keep-alive",
			},
		});
	} catch (error) {
		console.error("Error:", error);
		return NextResponse.json(
			{ error: "Internal Server Error" },
			{ status: 500 },
		);
	}
}
