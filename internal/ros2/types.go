// Package ros2 provides the small, read-only part of the ROS 2 C API used by
// the exporter. Each Client owns a separate ROS context and DDS domain.
package ros2

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrUnavailable means that this binary was built without native ROS support.
var ErrUnavailable = errors.New("native ROS 2 support is unavailable: build on Linux with CGO_ENABLED=1 and -tags ros2 against the sourced ROS 2 installation")

type Config struct {
	Name      string
	DomainID  int
	Namespace string
}

type Subscription struct {
	Key     string
	Topic   string
	Type    string
	Latched bool
}

type Message struct {
	Key string
	// Data contains copied Go values and no references to ROS-managed memory.
	// The transport transfers ownership; callers must treat it as immutable.
	Data map[string]any
	// Error identifies a recoverable failure of this source, such as a
	// message exceeding the decoder limits. Other sources remain usable.
	Error error
}

var messageTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*/msg/[A-Za-z][A-Za-z0-9_]*$`)
var namespace = regexp.MustCompile(`^(/[A-Za-z_][A-Za-z0-9_]*)*$`)

func validateConfig(c Config) error {
	if c.DomainID < 0 || c.DomainID > 232 {
		return fmt.Errorf("ROS domain ID %d must be between 0 and 232", c.DomainID)
	}
	if c.Namespace != "/" && !namespace.MatchString(strings.TrimSuffix(c.Namespace, "/")) {
		return fmt.Errorf("invalid ROS namespace %q", c.Namespace)
	}
	return nil
}

func validateSubscription(s Subscription) error {
	if s.Key == "" || s.Topic == "" || strings.ContainsRune(s.Key+s.Topic, '\x00') {
		return errors.New("subscription key and topic must be nonempty and contain no NUL bytes")
	}
	if !messageTypePattern.MatchString(s.Type) {
		return fmt.Errorf("invalid ROS message type %q: expected package/msg/Message", s.Type)
	}
	return nil
}

func nodeName(name string) string {
	return "autoware_exporter_" + strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, name)
}
