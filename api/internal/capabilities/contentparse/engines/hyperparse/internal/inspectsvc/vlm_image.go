package inspectsvc

import "context"

// Single-image pipeline prompts, kept provider-neutral for open-source use.

const vlmImageCaptionInstruction = `You are a document image captioning assistant. Write a short description in 1 to 4 sentences, using the source document language when it is clear.
- For organization logos or marks: describe the visual features and visible text.
- For medical, microscopy, or pathology images: describe the image type and visible structures.
- For ordinary illustrations or photos: summarize the visible scene.
Do not output JSON, Markdown fences, or restate these instructions.`

const vlmImageTextInstruction = `You are a document image OCR assistant. Extract only the visible text from the image in reading order as plain text.
- Do not describe, explain, or summarize the image.
- Preserve paragraphs and line breaks when possible.
- If there is almost no readable text, return an empty string.`

const vlmSidebarTextInstruction = `You are reading the narrow right sidebar of a bill or form page. Extract only the visible text, in top-to-bottom reading order, as plain text.
- Do not summarize, describe the image, or add explanations.
- Do not omit small text such as phone numbers, email addresses, postal addresses, account identifiers, customer service details, complaints, payment options, or margin notes.
- Preserve section headings such as Customer service, Complaints, or Payment options.
- Preserve line breaks when possible; if there are multiple sections, keep them separated.
- If there is almost no readable text, return an empty string.`

const vlmImageStructuredInstruction = `You are a document layout extraction assistant. Given an image, possibly a screenshot of a document page, extract structured content and return JSON.
Output JSON only, with no Markdown fences:
{"chunks":[{"type":"paragraph|heading|table|formula|image|list_item|kv|annotation|other","text":"...","order":0}]}

Requirements:
1) Output chunks in reading order.
2) For tables, prefer a payload grid; otherwise use a GFM Markdown pipe table. The server will normalize it to HTML with cell ids.
3) For formulas, use "formula:<expression>|<description>".
4) For handwritten notes, edits, circles, or margin annotations, output a separate chunk with type=annotation and preserve the original text as much as possible.
5) Do not output a whole-page summary or generic image description; extract the content itself.`

func callDashscopeVLMImageCaption(dataURL string) (text string, model string, err error) {
	return callDashscopeVLMImageCaptionContext(context.Background(), dataURL)
}

func callDashscopeVLMImageCaptionContext(ctx context.Context, dataURL string) (text string, model string, err error) {
	parts := []map[string]any{
		{"type": "text", "text": vlmImageCaptionInstruction},
		{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
	}
	return dashscopeChatCompletionWithModelContext(ctx, parts, VLMModelFast())
}

func callDashscopeVLMImageText(dataURL string) (text string, model string, err error) {
	return callDashscopeVLMImageTextContext(context.Background(), dataURL)
}

func callDashscopeVLMImageTextContext(ctx context.Context, dataURL string) (text string, model string, err error) {
	parts := []map[string]any{
		{"type": "text", "text": vlmImageTextInstruction},
		{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
	}
	return dashscopeChatCompletionWithModelContext(ctx, parts, VLMModelFast())
}

func callDashscopeVLMImageSidebarText(dataURL string) (text string, model string, err error) {
	return callDashscopeVLMImageSidebarTextContext(context.Background(), dataURL)
}

func callDashscopeVLMImageSidebarTextContext(ctx context.Context, dataURL string) (text string, model string, err error) {
	parts := []map[string]any{
		{"type": "text", "text": vlmSidebarTextInstruction},
		{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
	}
	return dashscopeChatCompletionContext(ctx, parts)
}

func callDashscopeVLMImageStructured(dataURL string) (text string, model string, err error) {
	return callDashscopeVLMImageStructuredContext(context.Background(), dataURL)
}

func callDashscopeVLMImageStructuredContext(ctx context.Context, dataURL string) (text string, model string, err error) {
	parts := []map[string]any{
		{"type": "text", "text": vlmImageStructuredInstruction},
		{"type": "image_url", "image_url": map[string]any{"url": dataURL}},
	}
	return dashscopeChatCompletionContext(ctx, parts)
}
