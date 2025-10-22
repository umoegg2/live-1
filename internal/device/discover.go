package device

import (
	"context"
	"errors"
)

var errSystemEnumerationUnsupported = errors.New("system enumeration unsupported")

// Discover aggregates capture devices by preferring system enumeration when
// available and falling back to manifest/default definitions otherwise. When
// includeDesktop is true a synthetic desktop entry is added if no explicit
// desktop device is discovered.
func Discover(ctx context.Context, manifestPath string, includeDesktop bool) ([]Info, error) {
	systemDevices, err := systemEnumerate(ctx)
	if err != nil && !errors.Is(err, errSystemEnumerationUnsupported) {
		return nil, err
	}

	devices := make([]Info, 0)
	devices = append(devices, systemDevices...)

	if manifestPath != "" {
		manifestDevices, err := (FileEnumerator{Path: manifestPath}).Enumerate(ctx)
		if err != nil {
			return nil, err
		}
		devices = append(devices, manifestDevices...)
	} else if len(devices) == 0 {
		fallback, err := (FileEnumerator{}).Enumerate(ctx)
		if err != nil {
			return nil, err
		}
		devices = append(devices, fallback...)
	}

	if includeDesktop && !hasKind(devices, KindDesktop) {
		devices = append(devices, Info{UID: "desktop-0", Kind: KindDesktop, Label: "Screen Share"})
	}

	return deduplicateInfos(devices), nil
}

func hasKind(devices []Info, kind Kind) bool {
	for _, dev := range devices {
		if dev.Kind == kind {
			return true
		}
	}
	return false
}

func deduplicateInfos(devices []Info) []Info {
	seen := make(map[string]struct{})
	out := make([]Info, 0, len(devices))
	for _, dev := range devices {
		if err := dev.Validate(); err != nil {
			continue
		}
		key := string(dev.Kind) + "::" + dev.UID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, dev)
	}
	return out
}
