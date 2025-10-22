package backend

import "live-1/internal/device"

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
