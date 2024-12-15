import { getStorageAdapter } from "@/lib/storageAdapter";
import { streamChatCompletions } from "@/server/chat.server";
import type { ChatHistory, ChatMessage } from "@/types/chat";
import { debounce } from "lodash";
import { create } from "zustand";

/**
 * Chat state management interface
 * @interface ChatStore
 */
interface ChatStore {
	currentChatId: string | null; // Currently selected chat ID
	chatHistories: ChatHistory[]; // All chat history records
	messageStreamingMap: Record<string, string>; // Streaming message status for each chat
	isLoadingMap: Record<string, boolean>; // Loading status for each chat
	setCurrentChatId: (id: string | null) => void; // Set current chat ID
	createChat: () => void; // Create new chat
	deleteChat: (id: string) => void; // Delete chat
	updateChatMessages: (chatId: string, messages: ChatMessage[]) => void; // Update chat messages
	updateChatTitle: (chatId: string, title: string) => void; // Update chat title
	clearAllChats: () => void; // Clear all chats
	loadChatsFromDisk: () => void; // Load chats from disk
	saveChatsToDisk: () => void; // Save chats to disk
	sendMessage: (message: ChatMessage) => void; // Send message
	updateChatTitleByContent: (chatId: string) => void; // Add new method
	isFirstOpen: boolean; // Flag indicating if it's first open
	updateChatTitleByFirstMessage: (chatId: string) => void; // Add new method
}

// Define the configuration interface for stream response handling
interface StreamResponseConfig {
	reader: ReadableStreamDefaultReader<Uint8Array>;
	chatId: string;
	messages: ChatMessage[];
	set: (
		partial: Partial<ChatStore> | ((state: ChatStore) => Partial<ChatStore>),
		replace?: boolean,
	) => void;
	onError?: (error: Error) => void;
	onComplete?: (fullMessage: string) => void;
}

// Refactored helper function with configuration object
const handleStreamResponse = async ({
	reader,
	chatId,
	messages,
	set,
	onError,
	onComplete,
}: StreamResponseConfig) => {
	const decoder = new TextDecoder();
	let fullMessage = "";

	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) break;

			const chunk = decoder.decode(value);
			const lines = chunk
				.split("\n")
				.filter((line) => line.trim() !== "" && line.trim() !== "data: [DONE]");

			for (const line of lines) {
				if (line.startsWith("data: ")) {
					try {
						const data = JSON.parse(line.slice(6));
						const content = data.choices[0]?.delta?.content;
						if (content) {
							fullMessage += content;
							// Update streaming message status
							set((state) => ({
								messageStreamingMap: {
									...state.messageStreamingMap,
									[chatId]: fullMessage,
								},
							}));
						}
					} catch (error) {
						console.error("Error parsing JSON:", error);
						onError?.(error as Error);
					}
				}
			}
		}

		// Add AI response to message list if we have a complete message
		if (fullMessage) {
			const assistantMessage: ChatMessage = {
				role: "assistant",
				content: fullMessage,
				timestamp: new Date().toISOString(),
			};

			set((state) => ({
				chatHistories: state.chatHistories.map((chat) => {
					if (chat.id === chatId) {
						return {
							...chat,
							messages: [...messages, assistantMessage],
						};
					}
					return chat;
				}),
				messageStreamingMap: { ...state.messageStreamingMap, [chatId]: "" },
			}));

			onComplete?.(fullMessage);
		}

		return fullMessage;
	} catch (error) {
		console.error("Error in stream handling:", error);
		onError?.(error as Error);
		throw error;
	}
};

/**
 * Create chat state management store
 */
export const useChatStore = create<ChatStore>()((set, get) => {
	const storageAdapter = getStorageAdapter();

	// Add helper function to update chat title based on content
	const updateChatTitleByContent = (chatId: string) => {
		const { chatHistories, isFirstOpen } = get();
		const chat = chatHistories.find((c) => c.id === chatId);

		// Only update title when the software is first opened
		if (!chat || !chat.messages.length || !isFirstOpen) return;

		// Get the first text message
		const firstTextMessage = chat.messages.find(
			(msg) => msg.role === "user" && !msg.fileType && msg.content.trim(),
		);

		if (firstTextMessage) {
			const newTitle =
				firstTextMessage.content.slice(0, 20) +
				(firstTextMessage.content.length > 20 ? "..." : "");

			set((state) => ({
				chatHistories: state.chatHistories.map((c) => {
					if (c.id === chatId) {
						return {
							...c,
							title: newTitle,
						};
					}
					return c;
				}),
			}));

			get().saveChatsToDisk();
		}
	};

	// Add helper function to update chat title based on first text message
	const updateChatTitleByFirstMessage = (chatId: string) => {
		const { chatHistories } = get();
		const chat = chatHistories.find((c) => c.id === chatId);

		if (!chat || !chat.messages.length) return;

		// Find first text message (no fileType)
		const firstTextMessage = chat.messages.find(
			(msg) => msg.role === "user" && !msg.fileType && msg.content.trim(),
		);

		if (firstTextMessage) {
			const newTitle =
				firstTextMessage.content.slice(0, 20) +
				(firstTextMessage.content.length > 20 ? "..." : "");

			set((state) => ({
				chatHistories: state.chatHistories.map((c) => {
					if (c.id === chatId) {
						return {
							...c,
							title: newTitle,
						};
					}
					return c;
				}),
			}));

			get().saveChatsToDisk();
		}
	};

	return {
		// Initial state
		currentChatId: null,
		chatHistories: [],
		messageStreamingMap: {},
		isLoadingMap: {},
		isFirstOpen: true, // Add first open flag

		/**
		 * Set current chat ID
		 * @param id Chat ID
		 */
		setCurrentChatId: (id) => {
			set({ currentChatId: id });
			get().saveChatsToDisk();
		},

		/**
		 * Create new chat
		 */
		createChat: () => {
			const newChat: ChatHistory = {
				id: Date.now().toString(),
				title: "New Chat",
				messages: [],
				createdAt: new Date().toISOString(),
			};
			set((state) => ({
				chatHistories: [newChat, ...state.chatHistories],
				currentChatId: newChat.id,
			}));
			get().saveChatsToDisk();
		},

		/**
		 * Delete specified chat
		 * @param id Chat ID to delete
		 */
		deleteChat: (id) => {
			set((state) => {
				const newHistories = state.chatHistories.filter(
					(chat) => chat.id !== id,
				);
				return {
					chatHistories: newHistories,
					currentChatId:
						state.currentChatId === id
							? (newHistories[0]?.id ?? null)
							: state.currentChatId,
				};
			});
			get().saveChatsToDisk();
		},

		/**
		 * Update message list for specified chat
		 * @param chatId Chat ID
		 * @param messages New message list
		 */
		updateChatMessages: (chatId, messages) => {
			set((state) => ({
				chatHistories: state.chatHistories.map((chat) =>
					chat.id === chatId ? { ...chat, messages } : chat,
				),
			}));
			get().saveChatsToDisk();
		},

		/**
		 * Update title for specified chat
		 * @param chatId Chat ID
		 * @param title New title
		 */
		updateChatTitle: (chatId, title) => {
			set((state) => ({
				chatHistories: state.chatHistories.map((chat) =>
					chat.id === chatId ? { ...chat, title } : chat,
				),
			}));
			get().saveChatsToDisk();
		},

		/**
		 * Clear all chats
		 */
		clearAllChats: () => {
			set({ chatHistories: [], currentChatId: null });
			get().saveChatsToDisk();
		},

		/**
		 * Load chat history from storage
		 */
		loadChatsFromDisk: async () => {
			try {
				const data = await storageAdapter.load();
				if (data) {
					set({
						chatHistories: data.chatHistories || [],
						currentChatId: data.currentChatId || null,
						isFirstOpen: true, // Reset to true each time loading
					});

					// Update titles for all chats after loading
					if (data.chatHistories) {
						for (const chat of data.chatHistories) {
							get().updateChatTitleByFirstMessage(chat.id);
						}
					}
				}
			} catch (error) {
				console.error("Failed to load chat history:", error);
			}
		},

		/**
		 * Save chat history to storage
		 * Using debounce to avoid frequent saves
		 */
		saveChatsToDisk: debounce(() => {
			const state = get();
			const data = {
				chatHistories: state.chatHistories,
				currentChatId: state.currentChatId,
			};
			storageAdapter.save(data);
		}, 1000),

		/**
		 * Send message and handle AI response
		 * @param message User message
		 */
		sendMessage: async (message: ChatMessage) => {
			const { currentChatId } = get();
			let chatId = currentChatId;

			// Check if already loading
			const isLoading = get().isLoadingMap[chatId || ""];
			if (isLoading) return;

			if (!chatId) {
				// Don't set title when creating new chat, wait for first message
				const newChat = {
					id: Date.now().toString(),
					title: "New Chat",
					messages: [],
					createdAt: new Date().toISOString(),
				};

				set((state) => ({
					chatHistories: [newChat, ...state.chatHistories],
					currentChatId: newChat.id,
				}));

				chatId = newChat.id;
			}

			const currentChat = get().chatHistories.find(
				(chat) => chat.id === chatId,
			);
			if (!currentChat) return;

			// Add user message to history
			const newMessages = [...currentChat.messages, message];

			// Update status immediately
			set((state) => ({
				chatHistories: state.chatHistories.map((chat) => {
					if (chat.id === chatId) {
						return {
							...chat,
							messages: newMessages,
						};
					}
					return chat;
				}),
			}));

			// Update chat title if this is a text message
			if (!message.fileType && message.content.trim()) {
				get().updateChatTitleByFirstMessage(chatId);
			}

			// If it's a file message and marked to skip AI response, return
			if (message.skipAIResponse) {
				return;
			}

			// Set loading status
			set((state) => ({
				isLoadingMap: { ...state.isLoadingMap, [chatId]: true },
				messageStreamingMap: { ...state.messageStreamingMap, [chatId]: "" },
			}));

			try {
				// Modify the format of the message sent to AI
				const messagesToSend = currentChat.messages.map((msg) => {
					if (msg.fileType?.includes("image/")) {
						// Handle image message
						let imageUrl = msg.content;
						if (!msg.content.startsWith("http")) {
							// If not a URL, convert to base64
							imageUrl = `data:${msg.fileType};base64,${msg.content}`;
						}

						return {
							role: msg.role,
							content: [
								{
									type: "image_url",
									image_url: {
										url: imageUrl,
									},
								},
							],
						};
					}
					// Handle normal text message
					return {
						role: msg.role,
						content: msg.content,
					};
				});

				// Handle the current message to send
				const currentMessageToSend = message.fileType?.includes("image/")
					? {
							role: message.role,
							content: [
								{
									type: "image_url",
									image_url: {
										url: message.content.startsWith("http")
											? message.content
											: `data:${message.fileType};base64,${message.content}`,
									},
								},
							],
						}
					: {
							role: message.role,
							content: message.content,
						};

				// If this is not a message to skip AI response, send request
				if (!message.skipAIResponse) {
					const reader = await streamChatCompletions({
						model: "gpt-4-vision-preview", // Use the model that supports images
						messages: [...messagesToSend, currentMessageToSend],
						stream: true,
						temperature: 1,
					});

					await handleStreamResponse({
						reader,
						chatId,
						messages: newMessages,
						set,
						onError: (error) => {
							console.error("Stream handling error:", error);
							set((state) => ({
								messageStreamingMap: {
									...state.messageStreamingMap,
									[chatId]:
										"Sorry, an error occurred while processing your message.",
								},
							}));
						},
						onComplete: (fullMessage) => {
							// Additional actions after completion if needed
							get().saveChatsToDisk();
						},
					});
				}
			} catch (error) {
				console.error("Failed to send message:", error);
				set((state) => ({
					messageStreamingMap: {
						...state.messageStreamingMap,
						[chatId]: "Sorry, failed to send message. Please try again later.",
					},
				}));
			} finally {
				set((state) => ({
					isLoadingMap: { ...state.isLoadingMap, [chatId]: false },
				}));

				if (!get().messageStreamingMap[chatId]) {
					get().saveChatsToDisk();
				}
			}
		},

		updateChatTitleByContent, // Export method
		updateChatTitleByFirstMessage,
	};
});
