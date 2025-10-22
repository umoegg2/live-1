package device

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kind represents the logical type of a capture device.
type Kind string

const (
	KindVideo   Kind = "video"
	KindAudio   Kind = "audio"
	KindDesktop Kind = "desktop"
)

// Info represents a single capture source that can be published.
type Info struct {
	UID   string `json:"uid"`
	Kind  Kind   `json:"kind"`
	Label string `json:"label"`
}

// Enumerator discovers available capture sources.
type Enumerator interface {
	Enumerate(ctx context.Context) ([]Info, error)
}

// FileEnumerator loads devices from a JSON manifest. When the manifest is not
// found, it returns a set of sensible defaults so the client can operate in
// environments where real device probing is not available (such as headless
// CI containers).
type FileEnumerator struct {
	Path string
}

// Enumerate implements the Enumerator interface.
func (e FileEnumerator) Enumerate(ctx context.Context) ([]Info, error) {
	if e.Path == "" {
		return defaultDevices(), nil
	}

	path := e.Path
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultDevices(), nil
		}
		return nil, fmt.Errorf("read devices manifest: %w", err)
	}

	var devices []Info
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, fmt.Errorf("parse devices manifest: %w", err)
	}

	filtered := make([]Info, 0, len(devices))
	for _, dev := range devices {
		if err := dev.Validate(); err != nil {
			continue
		}
		filtered = append(filtered, dev)
	}
	if len(filtered) == 0 {
		return defaultDevices(), nil
	}
	return filtered, nil
}

// Validate ensures the Info describes a usable device.
func (i Info) Validate() error {
	if strings.TrimSpace(i.UID) == "" {
		return fmt.Errorf("uid cannot be empty")
	}
	if strings.TrimSpace(string(i.Kind)) == "" {
		return fmt.Errorf("kind cannot be empty")
	}
	switch i.Kind {
	case KindVideo, KindAudio, KindDesktop:
	default:
		return fmt.Errorf("unsupported kind %q", i.Kind)
	}
	if strings.TrimSpace(i.Label) == "" {
		return fmt.Errorf("label cannot be empty")
	}
	return nil
}

func defaultDevices() []Info {
	return []Info{
		{UID: "camera-0", Kind: KindVideo, Label: "Default Camera"},
		{UID: "microphone-0", Kind: KindAudio, Label: "Default Microphone"},
		{UID: "desktop-0", Kind: KindDesktop, Label: "Screen Share"},
	}
}
