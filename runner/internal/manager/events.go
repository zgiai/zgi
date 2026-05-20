package manager

import "time"

// InstallEvent describes an installation stage update.
type InstallEvent struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Stage     string    `json:"stage"`
	Status    string    `json:"status"`
	Checksum  string    `json:"checksum,omitempty"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
