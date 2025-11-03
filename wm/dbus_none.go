//go:build !linux && !openbsd && !freebsd && !netbsd && !darwin

package wm

func RegisterService(obj interface{}, path, iface string) error {
	return nil
}
