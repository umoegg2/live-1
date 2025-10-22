//go:build windows

package device

import (
	"context"

	"github.com/pion/mediadevices"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
)

func systemEnumerate(ctx context.Context) ([]Info, error) {
	_ = ctx
	raw := mediadevices.EnumerateDevices()
	infos := make([]Info, 0, len(raw))
	for _, dev := range raw {
		info := Info{UID: dev.DeviceID, Label: dev.Label}
		switch dev.Kind {
		case mediadevices.VideoInput:
			info.Kind = KindVideo
		case mediadevices.AudioInput:
			info.Kind = KindAudio
		default:
			continue
		}
		if err := info.Validate(); err != nil {
			continue
		}
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return nil, nil
	}
	return infos, nil
}
