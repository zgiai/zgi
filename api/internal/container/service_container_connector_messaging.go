package container

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/dingtalk"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/ilink"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/wecom"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/weixin"
)

func (c *ServiceContainer) messagingDrivers() []providers.Driver {
	store := messaging.NewStore(c.db, c.getConnectorCredentialCipher())
	d, err := dingtalk.New(nil, true)
	if err != nil {
		panic("initialize DingTalk Connector contract")
	}
	w, err := wecom.NewWithHandleKey(nil, true, store, c.config.Connectors.CredentialKeys[c.config.Connectors.CredentialActiveKeyID])
	if err != nil {
		panic("initialize WeCom Connector contract")
	}
	x, err := weixin.New(nil, true, ilink.DefaultAppID, store)
	if err != nil {
		panic("initialize WeChat Connector protocol")
	}
	return []providers.Driver{d, w, x}
}
func (c *ServiceContainer) messagingAdapters() []onboarding.ProtocolAdapter {
	result := []onboarding.ProtocolAdapter{}
	for _, id := range []string{"dingtalk", "wecom", "weixin"} {
		a := onboarding.DefaultMessagingProtocol(id)
		var adapter onboarding.ProtocolAdapter
		var err error
		switch id {
		case "dingtalk":
			adapter, err = onboarding.NewDingTalkAdapter(nil, a)
		case "wecom":
			adapter, err = onboarding.NewWeComAdapter(nil, a)
		case "weixin":
			adapter, err = onboarding.NewWeixinAdapter(nil, a)
		}
		if err != nil {
			panic("initialize messaging Connector registration protocol")
		}
		result = append(result, adapter)
	}
	return result
}
