package whip

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"

	"live-1/internal/config"
	"live-1/internal/device"
)

// Session represents an active publishing session.
type Session interface {
	Stop(ctx context.Context)
}

// Manager is responsible for creating and managing WHIP sessions for tracks.
type Manager struct {
	baseURL    string
	secret     string
	streamKeys config.StreamKeyMapping
	client     *http.Client
	logger     *logrus.Logger
}

// NewManager constructs a Manager instance.
func NewManager(baseURL, secret string, keys config.StreamKeyMapping, httpClient *http.Client, logger *logrus.Logger) *Manager {
	if logger == nil {
		logger = logrus.StandardLogger()
	}
	return &Manager{
		baseURL:    strings.TrimRight(baseURL, "&"),
		secret:     secret,
		streamKeys: keys,
		client:     httpClient,
		logger:     logger,
	}
}

func (m *Manager) streamKeyFor(kind device.Kind) (string, error) {
	switch kind {
	case device.KindVideo:
		return m.streamKeys.Video, nil
	case device.KindDesktop:
		return m.streamKeys.Desktop, nil
	case device.KindAudio:
		return m.streamKeys.Audio, nil
	default:
		return "", fmt.Errorf("no stream key for kind %q", kind)
	}
}

func (m *Manager) buildEndpoint(streamKey string) (string, error) {
	if streamKey == "" {
		return "", fmt.Errorf("stream key cannot be empty")
	}

	baseURL, err := url.Parse(m.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse whip base url: %w", err)
	}

	query := baseURL.Query()
	query.Set("stream", streamKey)
	query.Set("secret", m.secret)
	baseURL.RawQuery = query.Encode()

	return baseURL.String(), nil
}
