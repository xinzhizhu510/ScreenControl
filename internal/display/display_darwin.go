//go:build darwin && cgo

package display

/*
#cgo CFLAGS: -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework CoreGraphics -framework AppKit -framework CoreFoundation -framework UniformTypeIdentifiers
#include "native_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

func enumerate() ([]target, error) {
	var count C.uint32_t
	if C.CGGetOnlineDisplayList(0, nil, &count) != C.kCGErrorSuccess {
		return nil, errors.New("macOS 无法获取显示器列表")
	}
	if count == 0 {
		return []target{}, nil
	}
	ids := make([]C.CGDirectDisplayID, count)
	if C.CGGetOnlineDisplayList(count, &ids[0], &count) != C.kCGErrorSuccess {
		return nil, errors.New("macOS 无法读取显示器列表")
	}
	helper := macHelper()
	result := make([]target, 0, count)
	for _, id := range ids[:count] {
		if C.CGDisplayIsActive(id) == 0 && C.CGDisplayIsAsleep(id) == 0 && C.CGDisplayIsInMirrorSet(id) == 0 {
			continue
		}
		var uuidBuffer [64]C.char
		var nameBuffer [512]C.char
		C.dc_display_info(id, &uuidBuffer[0], C.size_t(unsafe.Sizeof(uuidBuffer)), &nameBuffer[0], C.size_t(unsafe.Sizeof(nameBuffer)))
		uuid := C.GoString(&uuidBuffer[0])
		name := C.GoString(&nameBuffer[0])
		if name == "" {
			name = "显示器"
		}

		entry := target{Monitor: Monitor{
			ID:       macID(id, uuid),
			Name:     name,
			Width:    int(C.CGDisplayPixelsWide(id)),
			Height:   int(C.CGDisplayPixelsHigh(id)),
			Primary:  C.CGDisplayIsMain(id) != 0,
			Internal: C.CGDisplayIsBuiltin(id) != 0,
		}}
		if runtime.GOARCH == "arm64" && !entry.Internal && uuid != "" && bool(C.dc_can_disconnect(id)) {
			entry.disconnect = func() (func() error, error) {
				return disconnectMacScreen(id, uuid)
			}
		}
		switch {
		case entry.Internal:
			entry.Reason = "内置屏幕不支持单独电源控制。"
		case runtime.GOARCH != "arm64":
			entry.Reason = "当前版本的 macOS 单屏控制适用于 Apple Silicon。"
		case helper == "":
			entry.Reason = "找不到应用随附的 DDC/CI 组件。"
		case uuid == "":
			entry.Reason = "系统未提供可用于单屏控制的标识。"
		default:
			entry.beforeOff = func() error {
				if !C.dc_keep_window_visible(id) {
					return errors.New("找不到可接管控制窗口的屏幕，已取消关屏")
				}
				return nil
			}
			entry.probe = func() (uint32, error) {
				output, err := runDDC(helper, uuid, "get", "standby")
				if err != nil {
					return 0, err
				}
				value, err := strconv.ParseUint(strings.TrimSpace(output), 10, 32)
				return uint32(value), err
			}
			entry.write = func(value uint32) error {
				_, err := runDDC(helper, uuid, "set", "standby", strconv.FormatUint(uint64(value), 10))
				return err
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

func disconnectMacScreen(id C.CGDirectDisplayID, uuid string) (func() error, error) {
	if C.CGDisplayIsActive(id) == 0 || !C.dc_can_disconnect(id) || !C.dc_keep_window_visible(id) {
		return nil, errors.New("不能断开这台显示器，请确认另一块屏幕可用并退出镜像或全屏")
	}
	if status := C.dc_set_display_connected(id, false); status != C.kCGErrorSuccess {
		return nil, fmt.Errorf("macOS 无法断开显示器 (%d)", status)
	}
	return func() error {
		var currentUUID [64]C.char
		var unusedName [1]C.char
		C.dc_display_info(id, &currentUUID[0], 64, &unusedName[0], 1)
		current := C.GoString(&currentUUID[0])
		if current != "" && current != uuid {
			return errors.New("显示器标识已变化，请重新插拔显示线")
		}
		if status := C.dc_set_display_connected(id, true); status != C.kCGErrorSuccess {
			return fmt.Errorf("macOS 无法重新连接显示器 (%d)，可重新插拔显示线", status)
		}
		return nil
	}, nil
}

func macID(id C.CGDirectDisplayID, uuid string) string {
	if uuid != "" {
		return uuid
	}
	return fmt.Sprintf("display-%d", uint32(id))
}

func macHelper() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	helper := filepath.Join(filepath.Dir(executable), "m1ddc")
	if info, err := os.Stat(helper); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
		return helper
	}
	return ""
}

func runDDC(helper, uuid string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args = append([]string{"display", "uuid=" + uuid}, args...)
	output, err := exec.CommandContext(ctx, helper, args...).Output()
	if err != nil {
		return "", errors.New("显示器没有接受 DDC/CI 电源请求")
	}
	return string(output), nil
}

func identify(identifiers []string) error {
	var count C.uint32_t
	if C.CGGetOnlineDisplayList(0, nil, &count) != C.kCGErrorSuccess {
		return errors.New("macOS 无法获取显示器列表")
	}
	if count == 0 {
		return nil
	}
	ids := make([]C.CGDirectDisplayID, count)
	if C.CGGetOnlineDisplayList(count, &ids[0], &count) != C.kCGErrorSuccess {
		return errors.New("macOS 无法读取显示器列表")
	}
	numbers := displayNumbers(identifiers)
	for _, id := range ids[:count] {
		var uuid, name [512]C.char
		C.dc_display_info(id, &uuid[0], 512, &name[0], 512)
		if number, found := numbers[macID(id, C.GoString(&uuid[0]))]; found {
			C.dc_identify_screen(id, C.uint(number))
		}
	}
	return nil
}
