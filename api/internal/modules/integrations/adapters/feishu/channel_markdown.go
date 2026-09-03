package feishu

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	// Leave headroom for Feishu's internal rich-text expansion. Measure the
	// actual doubly JSON-encoded request, not rune count (Chinese and escapes
	// have very different byte costs). Text has a separate 150 KB limit.
	maxChannelPostBytes = 28 * 1024
	maxChannelTextBytes = 150 * 1024
)

var feishuMentionTag = regexp.MustCompile(`(?is)<\s*/?\s*at\b[^>]*>`)

type channelMessageBody struct {
	ReceiveID string `json:"receive_id"`
	MsgType   string `json:"msg_type"`
	Content   string `json:"content"`
	UUID      string `json:"uuid"`
}

type feishuPostNode struct {
	Tag      string `json:"tag"`
	Text     string `json:"text,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	UserName string `json:"user_name,omitempty"`
	Href     string `json:"href,omitempty"`
}

type feishuPost struct {
	Title   string             `json:"title,omitempty"`
	Content [][]feishuPostNode `json:"content"`
}

// sendChannelMessage is deliberately separate from the public integration
// action: DigitalHuman content is Markdown, whereas that action promises text.
// History and outbox payloads retain the original model output.
func (adapter *ChannelAdapter) sendChannelMessage(ctx context.Context, region, token string, request channelruntime.DeliveryRequest) (string, responseMeta, error) {
	if !validFeishuToken(strings.TrimSpace(request.ExternalConversationID)) {
		return "", responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu channel recipient is invalid", nil)
	}
	body, err := buildChannelMessage(request, false)
	if err != nil {
		return "", responseMeta{}, err
	}
	var response feishuMessageData
	meta, err := adapter.provider.client.sendMessage(ctx, region, token, "chat_id", body, &response)
	if err != nil && body.MsgType == "post" && feishuContentRejected(meta) {
		// Only an explicit content rejection proves that the first send did
		// not succeed. Auth errors, DLP rejection, timeouts and invalid success
		// responses must never result in a second send with another format.
		body, buildErr := buildChannelMessage(request, true)
		if buildErr != nil {
			return "", meta, buildErr
		}
		response = feishuMessageData{}
		fallbackMeta, fallbackErr := adapter.provider.client.sendMessage(ctx, region, token, "chat_id", body, &response)
		fallbackMeta.Attempts += meta.Attempts
		meta, err = fallbackMeta, fallbackErr
	}
	return strings.TrimSpace(response.MessageID), meta, err
}

func feishuContentRejected(meta responseMeta) bool {
	if meta.Diagnostics.ErrorCode == "230025" { // Explicit content size rejection.
		return true
	}
	field := meta.Diagnostics.InvalidField
	return meta.Diagnostics.ErrorCode == "230001" &&
		(field == "content" || strings.HasPrefix(field, "content.") || strings.HasPrefix(field, "content["))
}

func buildChannelMessage(request channelruntime.DeliveryRequest, plain bool) (channelMessageBody, error) {
	mentions := channelPostMentions(request.Mentions)
	messageType := "post"
	var content any
	if plain {
		messageType = "text"
		// Neutralize the model's mention tags BEFORE adding trusted mentions.
		// Otherwise raw <at> in generated HTML/code can notify arbitrary users.
		body := feishuMentionTag.ReplaceAllStringFunc(feishuPlainText(request.Text), func(tag string) string {
			return strings.NewReplacer("<", "＜", ">", "＞").Replace(tag)
		})
		content = struct {
			Text string `json:"text"`
		}{Text: renderFeishuDeliveryText(body, trustedDeliveryMentions(mentions))}
	} else {
		rows := make([][]feishuPostNode, 0, 2)
		if len(mentions) > 0 {
			rows = append(rows, mentions)
		}
		// Feishu requires md to occupy its own row. Native mentions are kept
		// in a separate row and never derived from generated Markdown.
		markdown := feishuMentionTag.ReplaceAllStringFunc(strings.TrimSpace(request.Text), html.EscapeString)
		rows = append(rows, []feishuPostNode{{Tag: "md", Text: markdown}})
		content = map[string]feishuPost{"zh_cn": {Content: rows}}
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return channelMessageBody{}, err
	}
	body := channelMessageBody{
		ReceiveID: strings.TrimSpace(request.ExternalConversationID), MsgType: messageType, Content: string(encoded),
		// Feishu only deduplicates for one hour; the durable outbox remains
		// authoritative. Distinct formats have distinct deterministic IDs.
		UUID: uuid.NewSHA1(request.DeliveryID, []byte("feishu:"+messageType)).String(),
	}
	wire, err := json.Marshal(body)
	if err != nil {
		return channelMessageBody{}, err
	}
	if !plain && len(wire) > maxChannelPostBytes {
		// Keep one durable intent == one provider message. Sending chunks
		// here would lose partial receipts and duplicate content on retry.
		return buildChannelMessage(request, true)
	}
	if plain && len(wire) > maxChannelTextBytes {
		return channelMessageBody{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu channel message exceeds the text size limit", nil)
	}
	return body, nil
}

func channelPostMentions(mentions []channelruntime.DeliveryMention) []feishuPostNode {
	result := make([]feishuPostNode, 0, min(len(mentions), 8))
	seen := make(map[string]bool)
	for _, mention := range mentions {
		id := strings.TrimSpace(mention.ExternalID)
		if !validFeishuToken(id) || strings.EqualFold(id, "all") || strings.EqualFold(id, "here") || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, feishuPostNode{Tag: "at", UserID: id, UserName: bounded(firstNonEmpty(strings.TrimSpace(mention.DisplayName), "成员"), 128)})
		if len(result) == 8 {
			break
		}
	}
	return result
}

func trustedDeliveryMentions(nodes []feishuPostNode) []channelruntime.DeliveryMention {
	result := make([]channelruntime.DeliveryMention, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, channelruntime.DeliveryMention{ExternalID: node.UserID, DisplayName: node.UserName})
	}
	return result
}

// feishuPlainText uses a CommonMark/GFM syntax tree rather than removing '*'
// and '_' with regexes: code, URLs, task checkboxes and tables must stay useful.
func feishuPlainText(markdown string) string {
	source := []byte(markdown)
	document := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(source))
	var result strings.Builder
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			switch value := node.(type) {
			case *ast.Link:
				result.WriteString(" (" + string(value.Destination) + ")")
			case *ast.Image:
				result.WriteString(" (" + string(value.Destination) + ")")
			case *ast.Heading, *ast.Paragraph, *ast.List, *ast.Blockquote:
				result.WriteString("\n\n")
			case *ast.ListItem, *extast.TableRow, *extast.TableHeader:
				result.WriteByte('\n')
			case *extast.TableCell:
				if node.NextSibling() != nil {
					result.WriteString(" | ")
				}
			}
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Text:
			result.Write(value.Segment.Value(source))
			if value.SoftLineBreak() || value.HardLineBreak() {
				result.WriteByte('\n')
			}
		case *ast.String:
			result.Write(value.Value)
		case *ast.AutoLink:
			result.Write(value.URL(source))
		case *ast.RawHTML:
			result.Write(value.Segments.Value(source))
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			result.Write(node.Lines().Value(source))
			if block, ok := node.(*ast.HTMLBlock); ok && block.HasClosure() {
				result.Write(block.ClosureLine.Value(source))
			}
			result.WriteString("\n\n")
			return ast.WalkSkipChildren, nil
		case *ast.ListItem:
			result.WriteString("• ")
		case *ast.ThematicBreak:
			result.WriteString("\n───\n")
		case *extast.TaskCheckBox:
			if value.IsChecked {
				result.WriteString("☑ ")
			} else {
				result.WriteString("☐ ")
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(result.String())
}

// Feishu returns posts either directly or nested under a locale. Read one
// locale only, so multilingual messages do not duplicate conversation history.
func normalizedPostText(raw string) string {
	var post feishuPost
	if json.Unmarshal([]byte(raw), &post) != nil {
		return ""
	}
	if post.Content == nil {
		var locales map[string]feishuPost
		if json.Unmarshal([]byte(raw), &locales) != nil {
			return ""
		}
		for _, locale := range []string{"zh_cn", "en_us", "ja_jp"} {
			if candidate, ok := locales[locale]; ok {
				post = candidate
				break
			}
		}
	}
	var result strings.Builder
	if post.Title != "" {
		result.WriteString(post.Title + "\n")
	}
	for _, row := range post.Content {
		for _, node := range row {
			switch node.Tag {
			case "text", "md", "code_block":
				result.WriteString(node.Text)
			case "a":
				result.WriteString(node.Text)
				if node.Href != node.Text {
					result.WriteString(" (" + node.Href + ")")
				}
			case "at":
				result.WriteString("@" + firstNonEmpty(node.UserName, node.UserID) + " ")
			case "img":
				result.WriteString("[image]")
			}
		}
		result.WriteByte('\n')
	}
	return bounded(strings.TrimSpace(result.String()), 4000)
}
