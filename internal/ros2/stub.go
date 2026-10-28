//go:build !linux || !cgo || !ros2

package ros2

import "time"

type Client struct{}

func Available() bool { return false }

func New(config Config) (*Client, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	return nil, ErrUnavailable
}

func (*Client) Subscribe(Subscription) error                { return ErrUnavailable }
func (*Client) Poll(time.Duration) ([]Message, error)       { return nil, ErrUnavailable }
func (*Client) PublisherCount(string) (int, error)          { return 0, ErrUnavailable }
func (*Client) PublisherFingerprint(string) (string, error) { return "", ErrUnavailable }
func (*Client) Close() error                                { return nil }
