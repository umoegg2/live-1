//go:build !windows

package whip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"live-1/internal/device"
)

type httpSession struct {
	endpoint string
	client   *http.Client
	log      *logrus.Entry
}

func (s *httpSession) Stop(ctx context.Context) {
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

func (m *Manager) Start(ctx context.Context, track device.Info) (Session, error) {
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

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read whip answer: %w", err)
	}

	entry := m.logger.WithFields(logrus.Fields{
		"track_uid":   track.UID,
		"track_kind":  track.Kind,
		"whip_stream": streamKey,
	})
	entry.Info("started WHIP session (placeholder)")

	return &httpSession{endpoint: endpoint, client: m.client, log: entry}, nil
}

func buildOffer(track device.Info) string {
	now := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("v=0\no=- %s 2 IN IP4 127.0.0.1\ns=WHIP Placeholder\nt=0 0\na=track:%s:%s\n", now, track.Kind, track.UID)
}
