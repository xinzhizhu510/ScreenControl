package display

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

type adapterID struct {
	low  uint32
	high int32
}

type displayEndpoint struct {
	adapter adapterID
	id      uint32
	mode    uint32
}

type displayPath struct {
	source                                                                    displayEndpoint
	sourceStatus                                                              uint32
	target                                                                    displayEndpoint
	output, rotation, scaling, refreshNumerator, refreshDenominator, scanline uint32
	available, targetStatus, flags                                            uint32
}

type displayMode struct {
	fields [16]uint32
}

type deviceInfoHeader struct {
	kind, size uint32
	adapter    adapterID
	id         uint32
}

var (
	getDisplayConfigSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	queryDisplayConfig    = user32.NewProc("QueryDisplayConfig")
	setDisplayConfig      = user32.NewProc("SetDisplayConfig")
	getDisplayDeviceInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")
)

const (
	allDisplayPaths    = 1
	activeDisplayPaths = 2
	pathActive         = 1
	noDisplayMode      = ^uint32(0)
)

func displayConfig(flags uintptr) ([]displayPath, []displayMode, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var pathCount, modeCount uint32
		status, _, _ := getDisplayConfigSizes.Call(flags, uintptr(unsafe.Pointer(&pathCount)), uintptr(unsafe.Pointer(&modeCount)))
		if status != 0 {
			return nil, nil, fmt.Errorf("无法读取 Windows 显示布局 (%d)", status)
		}
		paths := make([]displayPath, pathCount)
		modes := make([]displayMode, modeCount)
		status, _, _ = queryDisplayConfig.Call(flags, uintptr(unsafe.Pointer(&pathCount)), uintptr(unsafe.Pointer(unsafe.SliceData(paths))),
			uintptr(unsafe.Pointer(&modeCount)), uintptr(unsafe.Pointer(unsafe.SliceData(modes))), 0)
		if status == 122 { // The layout changed after the buffer sizes were read.
			continue
		}
		if status != 0 {
			return nil, nil, fmt.Errorf("无法读取 Windows 显示布局 (%d)", status)
		}
		return paths[:pathCount], modes[:modeCount], nil
	}
	return nil, nil, errors.New("显示布局正在变化，请重试")
}

func applyDisplayConfig(paths []displayPath, modes []displayMode) error {
	const supplied = 0x20
	const validate = 0x40
	const apply = 0x80
	const allowChanges = 0x400
	var usedModes []displayMode
	indices := make(map[uint32]uint32)
	for i := range paths {
		for _, index := range []*uint32{&paths[i].source.mode, &paths[i].target.mode} {
			if *index == noDisplayMode {
				continue
			}
			if uint64(*index) >= uint64(len(modes)) {
				return errors.New("Windows 返回了无效的显示模式")
			}
			newIndex, found := indices[*index]
			if !found {
				newIndex = uint32(len(usedModes))
				usedModes = append(usedModes, modes[*index])
				indices[*index] = newIndex
			}
			*index = newIndex
		}
	}
	for _, action := range []uintptr{validate, apply} {
		status, _, _ := setDisplayConfig.Call(uintptr(len(paths)), uintptr(unsafe.Pointer(unsafe.SliceData(paths))),
			uintptr(len(usedModes)), uintptr(unsafe.Pointer(unsafe.SliceData(usedModes))), supplied|allowChanges|action)
		if status != 0 {
			return fmt.Errorf("Windows 无法更新显示布局 (%d)", status)
		}
	}
	return nil
}

func sourceName(source displayEndpoint) string {
	var device struct {
		header deviceInfoHeader
		name   [32]uint16
	}
	device.header = deviceInfoHeader{1, uint32(unsafe.Sizeof(device)), source.adapter, source.id}
	status, _, _ := getDisplayDeviceInfo.Call(uintptr(unsafe.Pointer(&device)))
	if status != 0 {
		return ""
	}
	return syscall.UTF16ToString(device.name[:])
}

func targetName(target displayEndpoint) string {
	var device struct {
		header          deviceInfoHeader
		flags, output   uint32
		vendor, product uint16
		connector       uint32
		friendly        [64]uint16
		path            [128]uint16
	}
	device.header = deviceInfoHeader{2, uint32(unsafe.Sizeof(device)), target.adapter, target.id}
	status, _, _ := getDisplayDeviceInfo.Call(uintptr(unsafe.Pointer(&device)))
	if status != 0 {
		return ""
	}
	return syscall.UTF16ToString(device.path[:])
}

func disconnectWindowsScreen(name string) (func() error, error) {
	paths, modes, err := displayConfig(activeDisplayPaths)
	if err != nil {
		return nil, err
	}
	index := -1
	for i, path := range paths {
		if sourceName(path.source) == name {
			if index >= 0 {
				return nil, errors.New("请先退出镜像显示，再断开这台显示器")
			}
			index = i
		}
	}
	if index < 0 || len(paths) < 2 {
		return nil, errors.New("无法定位显示器，或没有可保留的显示器")
	}
	removed := paths[index]
	identity := targetName(removed.target)
	if identity == "" {
		return nil, errors.New("无法唯一识别显示器，已取消断开")
	}
	paths = append(paths[:index], paths[index+1:]...)
	primary := 0
	for _, path := range paths {
		mode := path.source.mode
		if mode != noDisplayMode && int(mode) < len(modes) && modes[mode].fields[7] == 0 && modes[mode].fields[8] == 0 {
			primary = -1
			break
		}
	}
	if primary >= 0 {
		// Rebase remaining source positions if the primary display was removed.
		mode := paths[primary].source.mode
		if mode != noDisplayMode && int(mode) < len(modes) {
			x, y := modes[mode].fields[7], modes[mode].fields[8]
			for i := range modes {
				if modes[i].fields[0] == 1 {
					modes[i].fields[7] -= x
					modes[i].fields[8] -= y
				}
			}
		}
	}
	if err := applyDisplayConfig(paths, modes); err != nil {
		return nil, err
	}
	return func() error { return reconnectWindowsScreen(removed, identity) }, nil
}

func reconnectWindowsScreen(saved displayPath, identity string) error {
	paths, modes, err := displayConfig(activeDisplayPaths)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if targetName(path.target) == identity {
			return nil
		}
	}
	candidates, _, err := displayConfig(allDisplayPaths)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if candidate.available == 0 || candidate.target.adapter != saved.target.adapter ||
			candidate.target.id != saved.target.id || targetName(candidate.target) != identity {
			continue
		}
		free := true
		for _, path := range paths {
			if path.source.adapter == candidate.source.adapter && path.source.id == candidate.source.id {
				free = false
				break
			}
		}
		if free {
			candidate.source.mode, candidate.target.mode = noDisplayMode, noDisplayMode
			candidate.flags |= pathActive
			return applyDisplayConfig(append(paths, candidate), modes)
		}
	}
	return errors.New("无法恢复显示器，请检查接线后重试")
}
