package display

import (
	"errors"
	"testing"
)

func TestPowerTargetsOnlySelectedDisplay(t *testing.T) {
	service := New()
	firstWrites := []uint32{}
	secondWrites := []uint32{}
	service.scan = func() ([]target, error) {
		return []target{testTarget("first", &firstWrites), testTarget("second", &secondWrites)}, nil
	}
	if _, err := service.SetDisplayPower("second", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetDisplayPower("missing", false); err == nil {
		t.Fatal("unknown display accepted a command")
	}
	if len(firstWrites) != 0 || len(secondWrites) != 1 || secondWrites[0] != 4 {
		t.Fatalf("unexpected power writes: first=%v, second=%v", firstWrites, secondWrites)
	}
}

func TestPoweredOffDisplayRemainsInListWhenConnectionDisappears(t *testing.T) {
	service := New()
	var writes []uint32
	connected := true
	service.scan = func() ([]target, error) {
		if connected {
			return []target{testTarget("external", &writes)}, nil
		}
		return nil, nil
	}
	if _, err := service.SetDisplayPower("external", false); err != nil {
		t.Fatal(err)
	}
	connected = false
	displays, err := service.ListDisplays()
	if err != nil {
		t.Fatal(err)
	}
	if len(displays) != 1 || displays[0].Connected || displays[0].CanPower || displays[0].Power != "unknown" {
		t.Fatalf("unexpected displays: %+v", displays)
	}
}

func TestPowerOffStopsIfControlWindowCannotStayVisible(t *testing.T) {
	service := New()
	var writes []uint32
	entry := testTarget("external", &writes)
	moveErr := errors.New("no usable screen")
	entry.beforeOff = func() error { return moveErr }
	service.scan = func() ([]target, error) { return []target{entry}, nil }

	if _, err := service.SetDisplayPower("external", false); !errors.Is(err, moveErr) {
		t.Fatalf("expected window error, got %v", err)
	}
	if len(writes) != 0 {
		t.Fatalf("unexpected power writes: %v", writes)
	}
}

func testTarget(id string, writes *[]uint32) target {
	return target{
		Monitor: Monitor{ID: id},
		probe:   func() (uint32, error) { return 1, nil },
		write: func(value uint32) error {
			*writes = append(*writes, value)
			return nil
		},
	}
}

func TestDisconnectedDisplayCanBeReconnected(t *testing.T) {
	service := New()
	connected := true
	external := target{Monitor: Monitor{ID: "external"}}
	external.disconnect = func() (func() error, error) {
		connected = false
		return func() error {
			connected = true
			return nil
		}, nil
	}
	service.scan = func() ([]target, error) {
		result := []target{{Monitor: Monitor{ID: "other"}}}
		if connected {
			result = append(result, external)
		}
		return result, nil
	}

	if err := service.SetDisplayConnection("external", false); err != nil {
		t.Fatal(err)
	}
	displays, err := service.ListDisplays()
	if err != nil || len(displays) != 2 || !displays[1].Disconnected || displays[1].ID != "external" {
		t.Fatalf("disconnected display missing: %+v, %v", displays, err)
	}
	if err := service.SetDisplayConnection("external", true); err != nil {
		t.Fatal(err)
	}
	displays, err = service.ListDisplays()
	if err != nil || len(displays) != 2 || !displays[1].Connected || displays[1].Disconnected {
		t.Fatalf("reconnected display missing: %+v, %v", displays, err)
	}
}

func TestCannotDisconnectLastDisplay(t *testing.T) {
	service := New()
	service.scan = func() ([]target, error) {
		return []target{{Monitor: Monitor{ID: "last"}, disconnect: func() (func() error, error) {
			t.Fatal("last display must remain connected")
			return nil, nil
		}}}, nil
	}
	if err := service.SetDisplayConnection("last", false); err == nil {
		t.Fatal("expected last display to be protected")
	}
}
