//go:build !windows

package device

import "context"

func systemEnumerate(ctx context.Context) ([]Info, error) {
	return nil, errSystemEnumerationUnsupported
}
