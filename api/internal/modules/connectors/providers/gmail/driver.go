// Package gmail preserves the Gmail app-password adapter and its existing wire contract.
package gmail

import "github.com/zgiai/zgi/api/internal/modules/connectors/providers/mailprotocol"

type Options = mailprotocol.Options
type Dialer = mailprotocol.Dialer
type Client = mailprotocol.Client

func New(options Options) (*Client, error) {
	options.Provider = "gmail"
	return mailprotocol.New(options)
}
