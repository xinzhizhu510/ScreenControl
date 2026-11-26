//go:build !windows && (!darwin || !cgo)

package display

import "errors"

func enumerate() ([]target, error) {
	return nil, errors.New("请使用适用于 macOS 或 Windows 的桌面应用")
}

func identify([]string) error {
	_, err := enumerate()
	return err
}
