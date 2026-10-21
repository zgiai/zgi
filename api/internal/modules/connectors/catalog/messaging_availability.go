package catalog

// MessagingAvailability describes supported setup methods, independently of
// whether an account has authorized or verified a connection.
type MessagingAvailability struct {
	Enabled           bool     `json:"enabled"`
	Methods           []string `json:"methods"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
}

func DefaultMessagingAvailability(id string) MessagingAvailability {
	d, ok := MessagingDefinition(id)
	if !ok {
		return MessagingAvailability{Methods: []string{}}
	}
	return MessagingAvailability{Enabled: true, Methods: append([]string{}, d.Setup.Methods...)}
}
