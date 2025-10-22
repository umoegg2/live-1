package session

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	"live-1/internal/device"
	"live-1/internal/whip"
)

type trackKey struct {
	Kind device.Kind
	UID  string
}

type entry struct {
	session whip.Session
	info    device.Info
}

// Manager maintains the lifecycle of active publishing sessions.
type Manager struct {
	whip   *whip.Manager
	mu     sync.Mutex
	active map[trackKey]entry
	logger *logrus.Logger
}

// NewManager creates a Manager.
func NewManager(whipManager *whip.Manager, logger *logrus.Logger) *Manager {
	if logger == nil {
		logger = logrus.StandardLogger()
	}
	return &Manager{
		whip:   whipManager,
		active: make(map[trackKey]entry),
		logger: logger,
	}
}

// StartTracks ensures the provided tracks are publishing. Tracks already running
// are ignored. Each track is deduplicated by (kind, uid).
func (m *Manager) StartTracks(ctx context.Context, tracks []device.Info) {
	deduped := deduplicate(tracks)
	for _, track := range deduped {
		key := trackKey{Kind: track.Kind, UID: track.UID}

		m.mu.Lock()
		if _, ok := m.active[key]; ok {
			m.mu.Unlock()
			m.logger.WithFields(logrus.Fields{
				"track_kind": track.Kind,
				"track_uid":  track.UID,
			}).Debug("track already active; skipping start")
			continue
		}
		m.mu.Unlock()

		session, err := m.whip.Start(ctx, track)
		if err != nil {
			m.logger.WithFields(logrus.Fields{
				"track_kind": track.Kind,
				"track_uid":  track.UID,
			}).WithError(err).Error("failed to start track")
			continue
		}

		m.mu.Lock()
		m.active[key] = entry{session: session, info: track}
		m.mu.Unlock()
	}
}

// StopAll terminates every active track session.
func (m *Manager) StopAll(ctx context.Context) {
	m.mu.Lock()
	entries := make([]entry, 0, len(m.active))
	for key, ent := range m.active {
		entries = append(entries, ent)
		delete(m.active, key)
	}
	m.mu.Unlock()

	for _, ent := range entries {
		ent.session.Stop(ctx)
	}
}

func deduplicate(tracks []device.Info) []device.Info {
	seen := make(map[trackKey]struct{})
	result := make([]device.Info, 0, len(tracks))
	for _, track := range tracks {
		if err := track.Validate(); err != nil {
			continue
		}
		key := trackKey{Kind: track.Kind, UID: track.UID}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, track)
	}
	return result
}

// Active returns a snapshot of the currently running tracks.
func (m *Manager) Active() []device.Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]device.Info, 0, len(m.active))
	for _, ent := range m.active {
		out = append(out, ent.info)
	}
	return out
}
