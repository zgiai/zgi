package announcement

const (
	NodeTypeAnnouncement = "announcement"

	defaultTimeoutDuration = 36
	defaultTimeoutUnit     = "hour"
	announcementURLPath    = "/n/"
)

type TimeoutConfig struct {
	Duration int    `json:"duration"`
	Unit     string `json:"unit"`
}

type NodeConfig struct {
	Content string        `json:"content"`
	Timeout TimeoutConfig `json:"timeout"`
	Title   string        `json:"title,omitempty"`
}

type CreateRuntimeAnnouncementParams struct {
	TenantID      string
	AppID         string
	WorkflowRunID string
	NodeID        string
	NodeTitle     string
	Config        NodeConfig
	Rendered      string
}

type AnnouncementPayload struct {
	ID           string `json:"id"`
	Token        string `json:"token"`
	NodeID       string `json:"node_id"`
	NodeTitle    string `json:"node_title,omitempty"`
	Content      string `json:"content"`
	ExpirationAt int64  `json:"expiration_at"`
	Expired      bool   `json:"expired"`
	URL          string `json:"url,omitempty"`
}

type RuntimeAnnouncement struct {
	Announcement *Announcement
	Payload      AnnouncementPayload
}
