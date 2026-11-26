package display

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type rectangle struct{ left, top, right, bottom int32 }

type monitorInfo struct {
	size   uint32
	full   rectangle
	work   rectangle
	flags  uint32
	device [32]uint16
}

type displayDevice struct {
	size      uint32
	name      [32]uint16
	label     [128]uint16
	flags     uint32
	id        [128]uint16
	deviceKey [128]uint16
}

type physicalMonitor struct {
	handle      uintptr
	description [128]uint16
}

type windowsScreen struct {
	target
	area rectangle
}

type message struct {
	window  uintptr
	id      uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	x, y    int32
	private uint32
}

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dxva2  = syscall.NewLazyDLL("dxva2.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	enumMonitors     = user32.NewProc("EnumDisplayMonitors")
	getMonitorInfo   = user32.NewProc("GetMonitorInfoW")
	enumDevices      = user32.NewProc("EnumDisplayDevicesW")
	getPhysicalCount = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	getPhysical      = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	destroyPhysical  = dxva2.NewProc("DestroyPhysicalMonitors")
	getVCP           = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	setVCP           = dxva2.NewProc("SetVCPFeature")
	createWindow     = user32.NewProc("CreateWindowExW")
	showWindow       = user32.NewProc("ShowWindow")
	destroyWindow    = user32.NewProc("DestroyWindow")
	sendMessage      = user32.NewProc("SendMessageW")
	peekMessage      = user32.NewProc("PeekMessageW")
	dispatchMessage  = user32.NewProc("DispatchMessageW")
	getStockObject   = gdi32.NewProc("GetStockObject")
)

const powerCode = 0xD6

type enumeration struct {
	screens []windowsScreen
	failed  bool
}

var enumerationMu sync.Mutex
var activeEnumeration *enumeration

var collectMonitor = syscall.NewCallback(func(handle, _, _, _ uintptr) uintptr {
	entry, err := windowsMonitor(handle)
	if err != nil {
		activeEnumeration.failed = true
		return 0
	}
	activeEnumeration.screens = append(activeEnumeration.screens, entry)
	return 1
})

func windowsScreens() ([]windowsScreen, error) {
	enumerationMu.Lock()
	defer enumerationMu.Unlock()

	result := &enumeration{}
	activeEnumeration = result
	ok, _, _ := enumMonitors.Call(0, 0, collectMonitor, 0)
	activeEnumeration = nil
	if ok == 0 || result.failed {
		return nil, errors.New("Windows 无法获取完整的显示器列表")
	}
	return result.screens, nil
}

func windowsMonitor(handle uintptr) (windowsScreen, error) {
	var info monitorInfo
	info.size = uint32(unsafe.Sizeof(info))
	ok, _, _ := getMonitorInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return windowsScreen{}, errors.New("无法读取显示器")
	}
	deviceName := syscall.UTF16ToString(info.device[:])
	record, count := monitorDevice(&info.device[0])
	name := syscall.UTF16ToString(record.label[:])
	if name == "" {
		name = "显示器 " + strings.TrimPrefix(deviceName, `\\.\DISPLAY`)
	}
	interfaceID := syscall.UTF16ToString(record.id[:])
	id := deviceName
	if interfaceID != "" {
		id += "|" + interfaceID
	}
	entry := target{Monitor: Monitor{
		ID:      id,
		Name:    name,
		Width:   int(info.full.right - info.full.left),
		Height:  int(info.full.bottom - info.full.top),
		Primary: info.flags&1 != 0,
	}}
	entry.disconnect = func() (func() error, error) { return disconnectWindowsScreen(deviceName) }
	if count != 1 || interfaceID == "" {
		entry.Reason = "无法将此屏幕与单独的物理显示器匹配。"
	} else {
		entry.probe = func() (value uint32, err error) {
			err = withPhysicalMonitor(handle, interfaceID, func(physical uintptr) error {
				ok, _, _ := getVCP.Call(physical, powerCode, 0, uintptr(unsafe.Pointer(&value)), 0)
				if ok == 0 {
					return errors.New("显示器未返回 DDC/CI 电源状态")
				}
				return nil
			})
			return
		}
		entry.write = func(value uint32) error {
			return withPhysicalMonitor(handle, interfaceID, func(physical uintptr) error {
				ok, _, _ := setVCP.Call(physical, powerCode, uintptr(value))
				if ok == 0 {
					return errors.New("显示器没有接受 DDC/CI 电源请求")
				}
				return nil
			})
		}
	}
	return windowsScreen{target: entry, area: info.work}, nil
}

func monitorDevice(deviceName *uint16) (displayDevice, int) {
	var selected displayDevice
	count := 0
	for index := uintptr(0); ; index++ {
		var device displayDevice
		device.size = uint32(unsafe.Sizeof(device))
		ok, _, _ := enumDevices.Call(uintptr(unsafe.Pointer(deviceName)), index, uintptr(unsafe.Pointer(&device)), 1)
		if ok == 0 {
			break
		}
		if device.flags&1 != 0 {
			selected = device
			count++
		}
	}
	runtime.KeepAlive(deviceName)
	return selected, count
}

func withPhysicalMonitor(handle uintptr, interfaceID string, operation func(uintptr) error) error {
	var info monitorInfo
	info.size = uint32(unsafe.Sizeof(info))
	ok, _, _ := getMonitorInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return errors.New("显示器连接已发生变化")
	}
	device, deviceCount := monitorDevice(&info.device[0])
	if deviceCount != 1 || syscall.UTF16ToString(device.id[:]) != interfaceID {
		return errors.New("显示器连接已发生变化")
	}
	var count uint32
	ok, _, _ = getPhysicalCount.Call(handle, uintptr(unsafe.Pointer(&count)))
	if ok == 0 || count != 1 {
		return errors.New("无法唯一定位物理显示器")
	}
	var physical physicalMonitor
	ok, _, _ = getPhysical.Call(handle, 1, uintptr(unsafe.Pointer(&physical)))
	if ok == 0 {
		return errors.New("无法访问物理显示器")
	}
	defer func() {
		destroyPhysical.Call(1, uintptr(unsafe.Pointer(&physical)))
	}()
	return operation(physical.handle)
}

func enumerate() ([]target, error) {
	screens, err := windowsScreens()
	if err != nil {
		return nil, err
	}
	result := make([]target, 0, len(screens))
	for _, screen := range screens {
		result = append(result, screen.target)
	}
	return result, nil
}

func identify(identifiers []string) error {
	screens, err := windowsScreens()
	if err != nil {
		return err
	}
	numbers := displayNumbers(identifiers)
	go showNumbers(screens, numbers)
	return nil
}

func showNumbers(screens []windowsScreen, numbers map[string]int) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	class, _ := syscall.UTF16PtrFromString("STATIC")
	font, _, _ := getStockObject.Call(17) // DEFAULT_GUI_FONT
	windows := make([]uintptr, 0, len(screens))
	defer func() {
		for _, window := range windows {
			destroyWindow.Call(window)
		}
	}()
	for _, screen := range screens {
		number, found := numbers[screen.ID]
		if !found {
			continue
		}
		title, _ := syscall.UTF16PtrFromString(fmt.Sprintf("显示器 %d", number))
		x := screen.area.left + (screen.area.right-screen.area.left-140)/2
		y := screen.area.top + (screen.area.bottom-screen.area.top-64)/2
		const exStyle = 0x08000000 | 0x00000080 | 0x00000008 | 0x00000020 // no activate, tool window, topmost, transparent
		const style = 0x80000000 | 0x00800000 | 0x00000200 | 0x00000001   // popup, border, center text
		window, _, _ := createWindow.Call(exStyle, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
			style, uintptr(x), uintptr(y), 140, 64, 0, 0, 0, 0)
		runtime.KeepAlive(title)
		if window != 0 {
			windows = append(windows, window)
			sendMessage.Call(window, 0x0030, font, 1) // WM_SETFONT
			showWindow.Call(window, 4)                // SW_SHOWNOACTIVATE
		}
	}
	runtime.KeepAlive(class)
	deadline := time.Now().Add(2 * time.Second)
	for len(windows) != 0 && time.Now().Before(deadline) {
		var msg message
		for {
			ok, _, _ := peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
		time.Sleep(15 * time.Millisecond)
	}
}
