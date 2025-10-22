package whip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"live-1/internal/config"
	"live-1/internal/device"
)

// Session represents a single active publishing session.
type Session struct {
	track    device.Info
	endpoint string
	answer   string
	client   *http.Client
	log      *logrus.Entry
	mu       sync.RWMutex
	stopped  bool
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

// Start initiates a WHIP publishing session for the given track.
func (m *Manager) Start(ctx context.Context, track device.Info) (*Session, error) {
	streamKey, err := m.streamKeyFor(track.Kind)
	if err != nil {
		return nil, err
	}

	endpoint, err := m.buildEndpoint(streamKey)
	if err != nil {
		return nil, err
	}

	offer := buildOffer(track)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(offer))
	if err != nil {
		return nil, fmt.Errorf("build whip request: %w", err)
	}
	req.Header.Set("Content-Type", "application/sdp")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send whip request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("whip request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	answerBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read whip answer: %w", err)
	}

	entry := m.logger.WithFields(logrus.Fields{
		"track_uid":   track.UID,
		"track_kind":  track.Kind,
		"whip_stream": streamKey,
	})
	entry.Info("started WHIP session")

	return &Session{
		track:    track,
		endpoint: endpoint,
		answer:   string(answerBytes),
		client:   m.client,
		log:      entry,
	}, nil
}

// Stop ends the session. WHIP specifies DELETE for teardown, but not all servers
// require it. The implementation attempts DELETE and logs failures without
// treating them as fatal errors.
func (s *Session) Stop(ctx context.Context) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.endpoint, nil)
	if err != nil {
		s.log.WithError(err).Warn("build whip teardown request failed")
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.WithError(err).Warn("whip teardown request failed")
		return
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.log.Info("stopped WHIP session")
	} else {
		s.log.WithField("status", resp.StatusCode).Warn("whip teardown returned non-success status")
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

func buildOffer(track device.Info) string {
	// For this reference implementation we produce a synthetic offer string.
	// Integrating with a real media stack can replace this implementation.
	now := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("v=0\no=- %s 2 IN IP4 127.0.0.1\ns=WHIP Placeholder\nt=0 0\na=track:%s:%s\n", now, track.Kind, track.UID)
}
