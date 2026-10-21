// Package ilink contains fixed protocol metadata shared by registration and
// business requests. The public bot client ID comes from Tencent's protocol.
package ilink

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"strconv"
)

const Origin = "https://ilinkai.weixin.qq.com"

// Public protocol identifier, not an account credential.
// https://github.com/Tencent/openclaw-weixin/blob/70ab695f6a1ca87da4102f857a452e2acb6b37cf/package.json
const DefaultAppID = "bot"

func Headers(appID, token string, post bool) http.Header {
	h := http.Header{"Ilink-App-Id": {appID}, "Ilink-App-Clientversion": {"65536"}}
	if post {
		var raw [4]byte
		_, _ = rand.Read(raw[:])
		uin := strconv.FormatUint(uint64(binary.BigEndian.Uint32(raw[:])), 10)
		h.Set("AuthorizationType", "ilink_bot_token")
		h.Set("X-WECHAT-UIN", base64.StdEncoding.EncodeToString([]byte(uin)))
	}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	return h
}
func BaseInfo() map[string]string {
	return map[string]string{"channel_version": "1.0.0", "bot_agent": "zgi/connector"}
}
