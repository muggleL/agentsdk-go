//go:build windows

package security

func supportsNoFollow() bool {
	return false
}

func openNoFollow(_ string) error {
	return nil
}
