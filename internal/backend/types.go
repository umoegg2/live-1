package backend

import (
	"strings"

	"live-1/internal/device"
)

// RegisterRequest is the payload sent to the backend when announcing available devices.
type RegisterRequest struct {
	ClientID string        `json:"client_id"`
	Devices  []device.Info `json:"devices"`
}

// CommandResponse represents the JSON payload returned by the backend when a
// control command is available.
type CommandResponse struct {
	Command string        `json:"command"`
	Tracks  []device.Info `json:"tracks"`
}

// IsZero reports whether the response contains a usable command payload.
func (c CommandResponse) IsZero() bool {
	if strings.TrimSpace(c.Command) != "" {
		return false
	}
	return len(c.Tracks) == 0
}
