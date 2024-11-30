import { API_KEY } from "@/constants";
import { API_CONFIG } from "@/lib/http";

/**
 * Send messages and get real-time response stream
 * @param messages Message list
 * @param options Request configuration options
 * @returns Returns a readable stream
 */
interface StreamChatCompletionsParams {
	messages: Record<string, any>[];
	model?: string;
	temperature?: number;
	presence_penalty?: number;
	stream?: boolean;
}

/**
 * Send messages and get real-time response stream
 * @param params Request parameters including messages and configuration options
 * @returns Returns a readable stream
 */
export const streamChatCompletions = async (
	params: StreamChatCompletionsParams,
) => {
	const { messages, ...options } = params;

	const response = await fetch(`${API_CONFIG.COMMON}/v1/chat/completions`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Authorization: API_KEY,
		},
		body: JSON.stringify({
			...options,
			model: options?.model || "gpt-4-vision-preview",
			messages,
			stream: true,
			temperature: options?.temperature || 1,
			max_tokens: 4096,
		}),
	});

	if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
	const reader = response.body?.getReader();
	if (!reader) throw new Error("No reader available");

	return reader;
};
