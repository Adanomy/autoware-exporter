package ros2

import (
	"errors"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	maxDecodedBytes   = 4 * 1024 * 1024
	maxDecodedNodes   = 65536
	maxArrayElements  = 4096
	maxMessageDepth   = 32
	maxFieldNameBytes = 512
)

// decodeBudget bounds the logical decoded payload, not the ROS middleware's
// allocation or exact Go heap usage. Costs conservatively include map/slice
// slots, primitive slots, and copied field-name/string bytes.
type decodeBudget struct {
	bytes int
	nodes int
}

func (b *decodeBudget) take(bytes, nodes int) error {
	if bytes < 0 || bytes > maxDecodedBytes-b.bytes {
		return errors.New("ROS message exceeds the approximate 4 MiB decoded payload limit")
	}
	if nodes < 0 || nodes > maxDecodedNodes-b.nodes {
		return errors.New("ROS message exceeds the 65536 decoded value/container node limit")
	}
	b.bytes += bytes
	b.nodes += nodes
	return nil
}

func checkMessageDepth(depth int) error {
	if depth > maxMessageDepth {
		return errors.New("ROS message exceeds maximum nesting depth of 32")
	}
	return nil
}

func checkArraySize(count uint64) error {
	if count > maxArrayElements {
		return fmt.Errorf("ROS array exceeds maximum of %d elements", maxArrayElements)
	}
	return nil
}

// utf16ByteLength computes the actual UTF-8 payload before allocating the Go
// result. Invalid surrogate pairs use the standard Unicode replacement rune.
func utf16ByteLength(units []uint16) int {
	length := 0
	for i := 0; i < len(units); i++ {
		r := rune(units[i])
		if r >= 0xD800 && r <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
			r = utf16.DecodeRune(r, rune(units[i+1]))
			i++
		} else if utf16.IsSurrogate(r) {
			r = utf8.RuneError
		}
		length += utf8.RuneLen(r)
	}
	return length
}
