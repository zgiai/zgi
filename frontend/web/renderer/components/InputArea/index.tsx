import { useChatStore } from "@/store/chat.store";
import type { ChatMessage } from "@/types/chat";
import {
	ChevronDown,
	FileText,
	LayoutGrid,
	Maximize,
	Send,
	Settings,
} from "lucide-react";
import type React from "react";
import { useRef, useState } from "react";

// Define allowed file types
const ALLOWED_FILE_TYPES = ["image/*", ".pdf", ".doc", ".docx", ".txt"].join(
	",",
);

const InputArea = () => {
	const [message, setMessage] = useState("");
	const [attachments, setAttachments] = useState<File[]>([]);
	const fileInputRef = useRef<HTMLInputElement>(null);
	const { sendMessage, isLoadingMap, currentChatId } = useChatStore();

	const isLoading = currentChatId ? isLoadingMap[currentChatId] : false;

	const handleSend = async () => {
		if (isLoading) return;

		try {
			const messages: ChatMessage[] = [];

			// 1. Process all attachments
			if (attachments.length > 0) {
				const filePromises = attachments.map((file) => {
					return new Promise<ChatMessage>((resolve) => {
						const reader = new FileReader();
						reader.onload = () => {
							const fileContent = reader.result as string;
							const base64Content = fileContent.split(",")[1];

							resolve({
								id: Date.now().toString(),
								role: "user",
								content: base64Content,
								fileType: file.type,
								fileName: file.name,
								timestamp: new Date().toISOString(),
								skipAIResponse: true,
							});
						};
						reader.readAsDataURL(file);
					});
				});

				// Wait for all files to be processed
				const fileMessages = await Promise.all(filePromises);
				messages.push(...fileMessages);
			}

			// 2. Add text message (if any)
			if (message.trim()) {
				messages.push({
					id: Date.now().toString(),
					role: "user",
					content: message.trim(),
					timestamp: new Date().toISOString(),
					skipAIResponse: false,
				});
			}

			// 3. Send messages if there are any
			if (messages.length > 0) {
				// Send all messages in sequence
				for (const msg of messages) {
					await sendMessage(msg);
				}

				// Clear input and attachments
				setMessage("");
				setAttachments([]);
			}
		} catch (error) {
			console.error("Failed to send message:", error);
		}
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			if (!isLoading) {
				// Add loading state check
				handleSend();
			}
		}
	};

	const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
		const files = Array.from(e.target.files || []);
		if (files.length > 0) {
			console.log("Selected files:", files); // Add log for debugging
			setAttachments((prev) => [...prev, ...files]);
		}
		// Reset input value, so the same file can be selected multiple times
		if (fileInputRef.current) {
			fileInputRef.current.value = "";
		}
	};

	const handleFileButtonClick = (e: React.MouseEvent) => {
		e.preventDefault();
		e.stopPropagation();
		fileInputRef.current?.click();
	};

	// Check if the file is an image
	const isImageFile = (file: File) => {
		return file.type.startsWith("image/");
	};

	return (
		<div
			className="absolute bottom-0 right-0 bg-white border-t border-gray-200 p-4"
			style={{ width: "calc(100% - 260px)" }}
		>
			<div className="max-w-3xl mx-auto">
				<div className="flex flex-col">
					{/* Attachment preview area */}
					{attachments.length > 0 && (
						<div className="mb-2">
							{attachments.map((file, index) => (
								<div
									key={`file-${file.name}-${index}`}
									className="inline-flex items-center mr-2 mb-2"
								>
									{isImageFile(file) ? (
										<div className="relative group">
											<img
												src={URL.createObjectURL(file)}
												alt={file.name}
												className="h-16 w-16 object-cover rounded"
											/>
											<button
												type="button"
												onClick={() =>
													setAttachments(
														attachments.filter((_, i) => i !== index),
													)
												}
												className="absolute -top-2 -right-2 bg-white rounded-full p-0.5 shadow-sm 
                                 text-gray-500 hover:text-red-500"
											>
												×
											</button>
										</div>
									) : (
										<div className="flex items-center bg-gray-100 rounded-lg p-2">
											<span className="truncate max-w-[200px]">
												{file.name}
											</span>
											<button
												type="button"
												onClick={() =>
													setAttachments(
														attachments.filter((_, i) => i !== index),
													)
												}
												className="ml-2 text-gray-500 hover:text-red-500"
											>
												×
											</button>
										</div>
									)}
								</div>
							))}
						</div>
					)}

					{/* Input area */}
					<div className="flex items-center bg-white border border-gray-300 rounded-lg">
						<textarea
							value={message}
							onChange={(e) => setMessage(e.target.value)}
							onKeyDown={handleKeyDown}
							placeholder="Enter your question. Press Enter to send, Shift+Enter for new line"
							className="flex-1 p-3 outline-none resize-none min-h-[40px] max-h-[200px]"
							rows={1}
						/>
					</div>

					{/* Bottom functionality area */}
					<div className="flex justify-between items-center mt-2 text-sm z-10">
						<div className="flex items-center gap-4">
							{/* File upload button */}
							<button
								type="button"
								onClick={handleFileButtonClick}
								className="flex items-center text-gray-500 hover:text-gray-600"
								title="Add attachment"
							>
								<FileText size={18} />
							</button>

							{/* Model selection dropdown */}
							<div className="flex items-center gap-1 text-gray-600">
								<img src="/gpt-icon.png" alt="GPT" className="w-4 h-4" />
								<span>GPT 4-Turbo</span>
								<ChevronDown size={14} className="text-gray-400" />
							</div>

							{/* Content safety protocol link */}
							<div className="text-gray-400 text-xs">
								<span>Please follow the </span>
								<a
									href="/safety-protocol"
									className="text-gray-500 hover:text-blue-500"
								>
									content safety protocol
								</a>
								<span>. No inappropriate content allowed.</span>
							</div>
						</div>

						{/* Right side functionality */}
						<div className="flex items-center gap-3">
							{/* Settings button */}
							<button
								type="button"
								className="text-gray-500 hover:text-gray-600"
								title="Settings"
							>
								<Settings size={18} />
							</button>

							{/* Format button */}
							<button
								type="button"
								className="text-gray-500 hover:text-gray-600"
								title="Format"
							>
								<LayoutGrid size={18} />
							</button>

							{/* Fullscreen button */}
							<button
								type="button"
								className="text-gray-500 hover:text-gray-600"
								title="Fullscreen"
							>
								<Maximize size={18} />
							</button>

							{/* Send button */}
							<button
								type="button"
								onClick={handleSend}
								className={`${
									isLoading
										? "bg-gray-400 cursor-not-allowed"
										: "bg-[#3b82f6] hover:bg-blue-600"
								} text-white px-4 py-1.5 rounded-lg flex items-center gap-2`}
								disabled={
									isLoading || (!message.trim() && attachments.length === 0)
								}
							>
								{isLoading ? (
									<>
										<div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
										<span>Send</span>
									</>
								) : (
									<>
										<Send size={16} />
										<span>Send</span>
									</>
								)}
							</button>
						</div>
					</div>

					{/* Hidden file upload input */}
					<input
						type="file"
						ref={fileInputRef}
						onChange={handleFileSelect}
						onClick={(e) => e.stopPropagation()}
						className="hidden"
						multiple
						accept={ALLOWED_FILE_TYPES}
					/>
				</div>
			</div>
		</div>
	);
};

export default InputArea;
