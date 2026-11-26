package display

import (
	"errors"
	"sync"
)

// Monitor describes a display as reported by the operating system.
type Monitor struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Primary       bool   `json:"primary"`
	Internal      bool   `json:"internal"`
	Connected     bool   `json:"connected"`
	CanPower      bool   `json:"canPower"`
	Power         string `json:"power"`
	Reason        string `json:"reason"`
	CanDisconnect bool   `json:"canDisconnect"`
	Disconnected  bool   `json:"disconnected"`
}

type target struct {
	Monitor
	probe      func() (uint32, error)
	write      func(uint32) error
	beforeOff  func() error
	disconnect func() (func() error, error)
}

type disconnectedDisplay struct {
	monitor   Monitor
	reconnect func() error
}

type Service struct {
	mu           sync.Mutex
	switchedOff  map[string]Monitor
	scan         func() ([]target, error)
	disconnected map[string]disconnectedDisplay
}

func New() *Service {
	return &Service{
		switchedOff:  make(map[string]Monitor),
		scan:         enumerate,
		disconnected: make(map[string]disconnectedDisplay),
	}
}

func (s *Service) ListDisplays() ([]Monitor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	monitors, err := s.scan()
	if err != nil {
		return nil, err
	}

	result := make([]Monitor, 0, len(monitors))
	for _, entry := range monitors {
		monitor := entry.Monitor
		delete(s.disconnected, monitor.ID)
		monitor.CanDisconnect = entry.disconnect != nil && len(monitors) > 1
		monitor.Power = "unknown"
		monitor.Connected = true
		if value, err := readPower(entry); err == nil {
			switch value {
			case 1:
				monitor.CanPower = entry.write != nil
				monitor.Power = "on"
				delete(s.switchedOff, monitor.ID)
			case 2, 3, 4, 5:
				monitor.CanPower = entry.write != nil
				monitor.Power = "off"
			default:
				monitor.Reason = "显示器返回了未知的电源模式。"
			}
		} else if entry.probe != nil {
			monitor.Reason = "读不到电源状态，请检查显示器的 DDC/CI 设置和接线。"
		}
		if _, ok := s.switchedOff[monitor.ID]; ok {
			s.switchedOff[monitor.ID] = monitor
		}
		result = append(result, monitor)
	}
	for id, monitor := range s.switchedOff {
		if _, disconnected := s.disconnected[id]; disconnected {
			continue
		}
		if containsMonitor(result, id) {
			continue
		}
		monitor.Connected = false
		monitor.CanPower = false
		monitor.Power = "unknown"
		monitor.Reason = "关屏后系统已无法访问这台显示器，请使用显示器电源键恢复。"
		result = append(result, monitor)
	}
	for _, entry := range s.disconnected {
		result = append(result, entry.monitor)
	}
	return result, nil
}

func (s *Service) SetDisplayConnection(id string, connected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if connected {
		entry, found := s.disconnected[id]
		if !found {
			return errors.New("这台显示器没有由本工具断开")
		}
		if err := entry.reconnect(); err != nil {
			return err
		}
		delete(s.disconnected, id)
		return nil
	}

	monitors, err := s.scan()
	if err != nil {
		return err
	}
	if len(monitors) < 2 {
		return errors.New("必须保留至少一块显示器")
	}
	for _, entry := range monitors {
		if entry.ID != id || entry.disconnect == nil {
			continue
		}
		reconnect, err := entry.disconnect()
		if err != nil {
			return err
		}
		monitor := entry.Monitor
		monitor.Disconnected = true
		monitor.Primary = false
		s.disconnected[id] = disconnectedDisplay{monitor: monitor, reconnect: reconnect}
		return nil
	}
	return errors.New("无法断开这台显示器，请刷新后重试")
}

func containsMonitor(monitors []Monitor, id string) bool {
	for _, monitor := range monitors {
		if monitor.ID == id {
			return true
		}
	}
	return false
}

func readPower(entry target) (uint32, error) {
	if entry.probe == nil {
		return 0, errors.New("这台显示器不支持单独电源控制")
	}
	return entry.probe()
}

func (s *Service) SetDisplayPower(id string, on bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	monitors, err := s.scan()
	if err != nil {
		return "", err
	}
	for _, entry := range monitors {
		if entry.ID != id {
			continue
		}
		value, err := readPower(entry)
		if err != nil || entry.write == nil {
			return "", errors.New("现在无法访问这台显示器的电源控制，请刷新后重试")
		}
		if on {
			if value == 1 {
				return "显示器已经开启。", nil
			}
			if value < 2 || value > 5 {
				return "", errors.New("无法确认这台显示器的电源状态")
			}
			if err := entry.write(1); err != nil {
				return "", err
			}
			if current, err := entry.probe(); err == nil && current == 1 {
				delete(s.switchedOff, id)
				return "显示器已开启。", nil
			}
			return "已发送开启指令，等待显示器反馈。", nil
		}
		if value != 1 {
			return "", errors.New("无法确认这台显示器处于开启状态")
		}
		if entry.beforeOff != nil {
			if err := entry.beforeOff(); err != nil {
				return "", err
			}
		}
		if err := entry.write(4); err != nil {
			return "", err
		}
		entry.Connected = true
		s.switchedOff[id] = entry.Monitor
		return "已发送关闭指令，正在刷新显示器状态。", nil
	}
	return "", errors.New("显示器已断开，请刷新列表")
}

func (s *Service) IdentifyDisplays(identifiers []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return identify(identifiers)
}

func displayNumbers(identifiers []string) map[string]int {
	numbers := make(map[string]int, len(identifiers))
	for index, id := range identifiers {
		numbers[id] = index + 1
	}
	return numbers
}
