package feishu

import (
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type apiRequest struct {
	method, path string
	query        url.Values
	body         any
	write        bool
}

// Paths and payloads are platform code, never arbitrary plugin/user OpenAPI.
// Input has already passed the operation's strict JSON Schema.
func operationRequest(operation string, args map[string]any) (apiRequest, error) {
	r := apiRequest{method: "GET", query: url.Values{}}
	t := func(key string) string { return providers.Text(args, key) }
	segment := func(key string) string { return url.PathEscape(t(key)) }
	messagePath := "/open-apis/im/v1/messages/" + segment("message_id")
	docPath := "/open-apis/docx/v1/documents/" + segment("document_id")
	commentPath := "/open-apis/drive/v1/files/" + segment("file_token") + "/comments"
	for _, key := range []string{"page_token", "start_time", "end_time"} {
		if value := t(key); value != "" {
			r.query.Set(key, value)
		}
	}
	if size := providers.Number(args, "page_size"); size > 0 {
		r.query.Set("page_size", strconv.Itoa(size))
	}
	content, _ := json.Marshal(map[string]any{"text": t("text")})
	commentContent := map[string]any{"elements": []any{map[string]any{"type": "text_run", "text_run": map[string]any{"text": t("text")}}}}
	textElements := []any{map[string]any{"text_run": map[string]any{"content": t("text")}}}
	switch operation {
	case "feishu.message.batch_send":
		r.method, r.path, r.write = "POST", "/open-apis/message/v4/batch_send/", true
		r.body = map[string]any{"open_ids": args["open_ids"], "msg_type": "text", "content": map[string]any{"text": t("text")}}
	case "feishu.message.system_send":
		r.method, r.path, r.write = "POST", "/open-apis/im/v1/messages", true
		r.query.Set("receive_id_type", "open_id")
		divider, _ := json.Marshal(map[string]any{"type": "divider", "params": map[string]any{"divider_text": map[string]any{"text": t("text")}}, "options": map[string]any{"need_rollup": false}})
		r.body = map[string]any{"receive_id": t("open_id"), "msg_type": "system", "content": string(divider)}
	case "feishu.bot.get":
		r.path = "/open-apis/bot/v3/info"
	case "feishu.chat.list":
		r.path = "/open-apis/im/v1/chats"
	case "feishu.chat.get":
		r.path = "/open-apis/im/v1/chats/" + segment("chat_id")
	case "feishu.message.get":
		r.path = messagePath
	case "feishu.message.list":
		r.path = "/open-apis/im/v1/messages"
		r.query.Set("container_id_type", "chat")
		r.query.Set("container_id", t("chat_id"))
	case "feishu.document.read":
		r.path = docPath + "/raw_content"
	case "feishu.document.blocks.list":
		r.path = docPath + "/blocks"
	case "feishu.file.metadata.get":
		r.method = "POST"
		r.path = "/open-apis/drive/v1/metas/batch_query"
		r.body = map[string]any{"request_docs": []any{map[string]any{"doc_token": t("file_token"), "doc_type": t("file_type")}}, "with_url": true}
	case "feishu.wiki.node.get":
		r.path = "/open-apis/wiki/v2/spaces/get_node"
		r.query.Set("token", t("token"))
	case "feishu.comment.list":
		r.path = commentPath
		r.query.Set("file_type", "docx")
	case "feishu.message.send":
		r.method = "POST"
		r.path = "/open-apis/im/v1/messages"
		r.query.Set("receive_id_type", t("receive_id_type"))
		r.body = map[string]any{"receive_id": t("receive_id"), "msg_type": "text", "content": string(content)}
		r.write = true
	case "feishu.message.reply":
		r.method = "POST"
		r.path = messagePath + "/reply"
		r.body = map[string]any{"msg_type": "text", "content": string(content)}
		r.write = true
	case "feishu.message.update":
		r.method = "PUT"
		r.path = messagePath
		r.body = map[string]any{"msg_type": "text", "content": string(content)}
		r.write = true
	case "feishu.document.blocks.append":
		r.method = "POST"
		r.path = docPath + "/blocks/" + segment("block_id") + "/children"
		r.body = map[string]any{"children": []any{map[string]any{"block_type": 2, "text": map[string]any{"elements": textElements}}}, "index": -1}
		r.write = true
	case "feishu.document.blocks.update":
		r.method = "PATCH"
		r.path = docPath + "/blocks/" + segment("block_id")
		r.body = map[string]any{"update_text_elements": map[string]any{"elements": textElements}}
		r.write = true
	case "feishu.document.blocks.convert":
		r.method = "POST"
		r.path = "/open-apis/docx/v1/documents/blocks/convert"
		r.body = map[string]any{"content_type": "markdown", "content": t("text")}
		r.write = true
	case "feishu.comment.create":
		r.method = "POST"
		r.path = commentPath
		r.query.Set("file_type", "docx")
		r.body = map[string]any{"is_whole": true, "reply_list": map[string]any{"replies": []any{map[string]any{"content": commentContent}}}}
		r.write = true
	case "feishu.comment.reply":
		r.method = "POST"
		r.path = commentPath + "/" + segment("comment_id") + "/replies"
		r.query.Set("file_type", "docx")
		r.body = map[string]any{"content": commentContent}
		r.write = true
	case "feishu.comment.update":
		r.method = "PUT"
		r.path = commentPath + "/" + segment("comment_id") + "/replies/" + segment("reply_id")
		r.query.Set("file_type", "docx")
		r.body = map[string]any{"content": commentContent}
		r.write = true
	case "feishu.comment.delete":
		r.method = "DELETE"
		r.path = commentPath + "/" + segment("comment_id") + "/replies/" + segment("reply_id")
		r.query.Set("file_type", "docx")
		r.write = true
	case "feishu.reaction.list":
		r.path = messagePath + "/reactions"
	case "feishu.reaction.add":
		r.method = "POST"
		r.path = messagePath + "/reactions"
		r.body = map[string]any{"reaction_type": map[string]any{"emoji_type": t("emoji_type")}}
		r.write = true
	case "feishu.reaction.remove":
		r.method = "DELETE"
		r.path = messagePath + "/reactions/" + segment("reaction_id")
		r.write = true
	case "feishu.pin.list":
		r.path = "/open-apis/im/v1/pins"
		r.query.Set("chat_id", t("chat_id"))
	case "feishu.pin.add":
		r.method = "POST"
		r.path = "/open-apis/im/v1/pins"
		r.body = map[string]any{"message_id": t("message_id")}
		r.write = true
	case "feishu.pin.remove":
		r.method = "DELETE"
		r.path = "/open-apis/im/v1/pins/" + segment("message_id")
		r.write = true
	case "feishu.chat.create":
		r.method = "POST"
		r.path = "/open-apis/im/v1/chats"
		r.query.Set("user_id_type", "open_id")
		r.body = map[string]any{"name": t("name"), "user_id_list": args["user_id_list"]}
		r.write = true
	case "feishu.chat.update":
		r.method = "PUT"
		r.path = "/open-apis/im/v1/chats/" + segment("chat_id")
		r.body = map[string]any{"name": t("name")}
		if v := t("description"); v != "" {
			r.body.(map[string]any)["description"] = v
		}
		r.write = true
	case "feishu.card.create", "feishu.card.update":
		cardData, _ := json.Marshal(map[string]any{"schema": "2.0", "body": map[string]any{"elements": []any{map[string]any{"tag": "markdown", "content": t("text")}}}})
		card := map[string]any{"type": "card_json", "data": string(cardData)}
		r.method = "POST"
		r.path = "/open-apis/cardkit/v1/cards"
		r.body = card
		r.write = true
		if operation == "feishu.card.update" {
			r.method = "PUT"
			r.path += "/" + segment("card_id")
			r.body = map[string]any{"card": card, "sequence": providers.Number(args, "sequence")}
		}
	case "feishu.contact.basic.get":
		r.path = "/open-apis/contact/v3/users/" + segment("user_id")
		r.query.Set("user_id_type", "open_id")
	default:
		return apiRequest{}, providers.Invalid()
	}
	return r, nil
}
