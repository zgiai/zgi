package onboarding

import "github.com/zgiai/zgi/api/internal/modules/connectors/providers/ilink"

// DefaultMessagingProtocol supplies public protocol parameters for every
// built-in messaging Connector. These are not user credentials. Exact QR
// hosts and WeCom's initial waiting state were checked against the official
// endpoints; credential verification is still required after authorization.
func DefaultMessagingProtocol(id string) ProtocolSettings {
	switch id {
	case "dingtalk":
		// DingTalk-Real-AI/dingtalk-openclaw-connector, device-auth-config.ts,
		// commit 0ccfba47e62dbb3078d066207de26a3df67019cc.
		return ProtocolSettings{Source: "DING_DWS_CLAW", QRHosts: []string{"open-dev.dingtalk.com"}}
	case "wecom":
		// WecomTeam/wecom-cli, QR session protocol,
		// commit 78c514b2afee7c0d3d7be715628478421f37ee63.
		return ProtocolSettings{Source: "wecom_cli_external", QRHosts: []string{"work.weixin.qq.com"}}
	case "weixin":
		return ProtocolSettings{AppID: ilink.DefaultAppID, QRHosts: []string{"liteapp.weixin.qq.com"}}
	default:
		return ProtocolSettings{}
	}
}
