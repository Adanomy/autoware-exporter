package ros2

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestDecodeBudgetRejectsBeforeLimitOverrun(t *testing.T) {
	for _, test := range []struct {
		name         string
		bytes, nodes int
		contains     string
	}{
		{"payload", maxDecodedBytes + 1, 0, "4 MiB"},
		{"nodes", 0, maxDecodedNodes + 1, "65536"},
		{"negative", -1, 0, "4 MiB"},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := decodeBudget{}
			if err := b.take(test.bytes, test.nodes); err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("take error=%v", err)
			}
			if b.bytes != 0 || b.nodes != 0 {
				t.Fatal("failed reservation consumed the budget")
			}
		})
	}
	budget := decodeBudget{}
	if err := budget.take(maxDecodedBytes, maxDecodedNodes); err != nil {
		t.Fatal(err)
	}
	if err := budget.take(1, 0); err == nil {
		t.Fatal("accepted bytes beyond budget")
	}
	if err := budget.take(0, 1); err == nil {
		t.Fatal("accepted node beyond budget")
	}
}

func TestDecodeBoundsAndUTF16Length(t *testing.T) {
	if err := checkArraySize(maxArrayElements); err != nil {
		t.Fatal(err)
	}
	if err := checkArraySize(maxArrayElements + 1); err == nil {
		t.Fatal("accepted oversized array")
	}
	if err := checkMessageDepth(maxMessageDepth); err != nil {
		t.Fatal(err)
	}
	if err := checkMessageDepth(maxMessageDepth + 1); err == nil {
		t.Fatal("accepted oversized nesting")
	}
	for _, units := range [][]uint16{{}, {'a', '"', '\\', '\n'}, {0x0410, 0x4e00}, {0xd83d, 0xde00}, {0xd800}, {0xdc00}, {0xd800, 'a'}} {
		want := len(string(utf16.Decode(units)))
		if got := utf16ByteLength(units); got != want {
			t.Errorf("units=%v byte length=%d want%d", units, got, want)
		}
	}
}
