//go:build linux && cgo && ros2 && ros2test

package ros2

import (
	"math"
	"strings"
	"testing"
)

func nativeMessage(t *testing.T, name string) *messageType {
	t.Helper()
	m, err := loadMessageType(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.close)
	return m
}

func TestNativeIntrospectionCopiesStringAndResets(t *testing.T) {
	m := nativeMessage(t, "std_msgs/msg/String")
	text := "quote\" slash\\ newline\n Русский 😀\x00suffix"
	if err := fixtureString(m, text); err != nil {
		t.Fatal(err)
	}
	first, err := m.read()
	if err != nil {
		t.Fatal(err)
	}
	if first["data"] != text {
		t.Fatalf("string=%q, want%q", first["data"], text)
	}
	if err := fixtureString(m, "replacement"); err != nil {
		t.Fatal(err)
	}
	second, err := m.read()
	if err != nil {
		t.Fatal(err)
	}
	if first["data"] != text || second["data"] != "replacement" {
		t.Fatal("decoded strings alias reusable C message storage")
	}
	m.reset()
	reset, err := m.read()
	if err != nil || reset["data"] != "" {
		t.Fatalf("reset=%v, error=%v", reset, err)
	}
	m.close()
	if first["data"] != text {
		t.Fatal("closing C message invalidated decoded string")
	}
	if _, err := m.read(); err == nil {
		t.Fatal("closed message accepted read")
	}
}

func TestNativeIntrospectionNonFiniteFloatIsMissing(t *testing.T) {
	m := nativeMessage(t, "std_msgs/msg/Float64")
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := fixtureDouble(m, value); err != nil {
			t.Fatal(err)
		}
		data, err := m.read()
		if err != nil {
			t.Fatal(err)
		}
		if data["data"] != nil {
			t.Fatalf("non-finite value=%v", data["data"])
		}
	}
	if err := fixtureDouble(m, 1.25); err != nil {
		t.Fatal(err)
	}
	data, err := m.read()
	if err != nil || data["data"] != 1.25 {
		t.Fatalf("finite data=%v error=%v", data, err)
	}
}

func TestNativeIntrospectionSequenceAndFixedArrays(t *testing.T) {
	m := nativeMessage(t, "std_msgs/msg/UInt8MultiArray")
	if err := fixtureBytes(m, []byte{0, 127, 255}); err != nil {
		t.Fatal(err)
	}
	first, err := m.read()
	if err != nil {
		t.Fatal(err)
	}
	array := first["data"].([]any)
	if len(array) != 3 || array[0] != float64(0) || array[2] != float64(255) {
		t.Fatalf("array=%v", array)
	}
	layout, ok := first["layout"].(map[string]any)
	if !ok {
		t.Fatalf("nested layout=%T", first["layout"])
	}
	if dim, ok := layout["dim"].([]any); !ok || len(dim) != 0 {
		t.Fatalf("empty sequence=%v", layout["dim"])
	}
	if err := fixtureBytes(m, []byte{9}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.read(); err != nil {
		t.Fatal(err)
	}
	if len(array) != 3 || array[2] != float64(255) {
		t.Fatal("decoded sequence aliases reusable C storage")
	}
	fixed := nativeMessage(t, "sensor_msgs/msg/Imu")
	if err := fixtureFixedDoubles(fixed, "orientation_covariance", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}); err != nil {
		t.Fatal(err)
	}
	data, err := fixed.read()
	if err != nil {
		t.Fatal(err)
	}
	covariance := data["orientation_covariance"].([]any)
	if len(covariance) != 9 || covariance[8] != float64(9) {
		t.Fatalf("fixed array=%v", covariance)
	}
}

func TestNativeIntrospectionRejectsOversizedAndInvalidString(t *testing.T) {
	m := nativeMessage(t, "std_msgs/msg/UInt8MultiArray")
	if err := fixtureBytes(m, make([]byte, maxArrayElements+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.read(); err == nil || !strings.Contains(err.Error(), "4096") {
		t.Fatalf("oversized array error=%v", err)
	}
	text := nativeMessage(t, "std_msgs/msg/String")
	if err := fixtureString(text, strings.Repeat("x", maxDecodedBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := text.read(); err == nil || !strings.Contains(err.Error(), "4 MiB") {
		t.Fatalf("oversized payload error=%v", err)
	}
	if err := fixtureString(text, string([]byte{0xff})); err != nil {
		t.Fatal(err)
	}
	if _, err := text.read(); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 error=%v", err)
	}
}

func TestNativeIntrospectionMissingLibrariesAndSymbol(t *testing.T) {
	if m, err := loadMessageType("not_installed_exporter_fixture/msg/Missing"); err == nil || m != nil {
		t.Fatalf("missing library: message=%v, error=%v", m, err)
	}
	if m, err := loadMessageType("std_msgs/msg/NotARealMessage"); err == nil || m != nil {
		t.Fatalf("missing symbol: message=%v, error=%v", m, err)
	}
	valid := nativeMessage(t, "std_msgs/msg/String")
	if _, err := valid.read(); err != nil {
		t.Fatalf("failed load poisoned next valid type: %v", err)
	}
}
