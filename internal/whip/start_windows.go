//go:build windows

package whip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/pion/interceptor"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	_ "github.com/pion/mediadevices/pkg/driver/screen"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"

	"live-1/internal/device"
)

type windowsSession struct {
	endpoint string
	client   *http.Client
	log      *logrus.Entry
	pc       *webrtc.PeerConnection
	tracks   []mediadevices.Track
	stopOnce sync.Once
}

func (s *windowsSession) Stop(ctx context.Context) {
	s.stopOnce.Do(func() {
		for _, track := range s.tracks {
			_ = track.Close()
		}

		if s.pc != nil {
			_ = s.pc.Close()
		}

		if s.endpoint == "" {
			return
		}

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
	})
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

	selector, err := buildSelector(track.Kind)
	if err != nil {
		return nil, err
	}

	mediaEngine := &webrtc.MediaEngine{}
	selector.Populate(mediaEngine)

	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		return nil, fmt.Errorf("register interceptors: %w", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	)

	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}

	entry := m.logger.WithFields(logrus.Fields{
		"track_uid":   track.UID,
		"track_kind":  track.Kind,
		"whip_stream": streamKey,
	})

	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		entry.WithField("state", state.String()).Debug("ice state changed")
	})

	mediaTracks, err := attachTrack(peerConnection, track, selector)
	if err != nil {
		_ = peerConnection.Close()
		return nil, err
	}

	cleanupTracks := func() {
		for _, t := range mediaTracks {
			_ = t.Close()
		}
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("create offer: %w", err)
	}
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("set local description: %w", err)
	}

	select {
	case <-ctx.Done():
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, ctx.Err()
	case <-gatherComplete:
	}

	localDesc := peerConnection.LocalDescription()
	if localDesc == nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("local description missing")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(localDesc.SDP))
	if err != nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("build whip request: %w", err)
	}
	req.Header.Set("Content-Type", "application/sdp")

	resp, err := m.client.Do(req)
	if err != nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("send whip request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("whip request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	answerBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("read whip answer: %w", err)
	}

	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: string(answerBytes)}); err != nil {
		cleanupTracks()
		_ = peerConnection.Close()
		return nil, fmt.Errorf("set remote description: %w", err)
	}

	entry.Info("started WHIP session")

	return &windowsSession{
		endpoint: endpoint,
		client:   m.client,
		log:      entry,
		pc:       peerConnection,
		tracks:   mediaTracks,
	}, nil
}

func attachTrack(pc *webrtc.PeerConnection, info device.Info, selector *mediadevices.CodecSelector) ([]mediadevices.Track, error) {
	switch info.Kind {
	case device.KindAudio:
		return createAudio(pc, info, selector)
	case device.KindVideo, device.KindDesktop:
		return createVideo(pc, info, selector)
	default:
		return nil, fmt.Errorf("unsupported track kind %q", info.Kind)
	}
}

func createAudio(pc *webrtc.PeerConnection, info device.Info, selector *mediadevices.CodecSelector) ([]mediadevices.Track, error) {
	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Audio: func(c *mediadevices.MediaTrackConstraints) {
			c.DeviceID = prop.String(info.UID)
			c.SampleRate = prop.Int(48000)
		},
		Codec: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("get user media: %w", err)
	}

	tracks := stream.GetAudioTracks()
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no audio tracks available for %s", info.UID)
	}

	return bindTracks(pc, tracks)
}

func createVideo(pc *webrtc.PeerConnection, info device.Info, selector *mediadevices.CodecSelector) ([]mediadevices.Track, error) {
	constraints := mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			if info.Kind != device.KindDesktop {
				c.DeviceID = prop.String(info.UID)
			}
			c.Width = prop.Int(1280)
			c.Height = prop.Int(720)
			c.FrameRate = prop.Float(30)
		},
		Codec: selector,
	}

	var (
		stream mediadevices.MediaStream
		err    error
	)
	if info.Kind == device.KindDesktop {
		stream, err = mediadevices.GetDisplayMedia(constraints)
	} else {
		stream, err = mediadevices.GetUserMedia(constraints)
	}
	if err != nil {
		return nil, fmt.Errorf("get video media: %w", err)
	}

	tracks := stream.GetVideoTracks()
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no video tracks available for %s", info.UID)
	}

	return bindTracks(pc, tracks)
}

func bindTracks(pc *webrtc.PeerConnection, tracks []mediadevices.Track) ([]mediadevices.Track, error) {
	bound := make([]mediadevices.Track, 0, len(tracks))
	for _, track := range tracks {
		sender, err := pc.AddTrack(track)
		if err != nil {
			for _, added := range bound {
				_ = added.Close()
			}
			return nil, fmt.Errorf("add track: %w", err)
		}

		go func(s *webrtc.RTPSender) {
			buf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := s.Read(buf); rtcpErr != nil {
					return
				}
			}
		}(sender)

		bound = append(bound, track)
	}
	return bound, nil
}

func buildSelector(kind device.Kind) (*mediadevices.CodecSelector, error) {
	var opts []mediadevices.CodecSelectorOption

	switch kind {
	case device.KindAudio:
		opusParams, err := opus.NewParams()
		if err != nil {
			return nil, fmt.Errorf("configure opus: %w", err)
		}
		opusParams.BitRate = 64_000
		opusParams.Latency = opus.Latency20ms
		opts = append(opts, mediadevices.WithAudioEncoders(&opusParams))
	case device.KindVideo, device.KindDesktop:
		vp8Params, err := vpx.NewVP8Params()
		if err != nil {
			return nil, fmt.Errorf("configure vp8: %w", err)
		}
		vp8Params.BitRate = 2_000_000
		vp8Params.KeyFrameInterval = 60
		opts = append(opts, mediadevices.WithVideoEncoders(&vp8Params))
	default:
		return nil, fmt.Errorf("unsupported track kind %q", kind)
	}

	return mediadevices.NewCodecSelector(opts...), nil
}
