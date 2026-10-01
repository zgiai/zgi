package container

import "testing"

func TestMessagingAdaptersAlwaysRegisteredWithoutDeploymentConfig(t *testing.T) {
	c := &ServiceContainer{}
	adapters := c.messagingAdapters()
	if len(adapters) != 3 {
		t.Fatal("missing default messaging adapters")
	}
	for i, id := range []string{"dingtalk", "wecom", "weixin"} {
		if adapters[i].ConnectorID() != id {
			t.Fatal("wrong messaging adapter")
		}
	}
}
