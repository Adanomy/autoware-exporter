// Package telemetry describes read-only ROS sources and immutable collection snapshots.
package telemetry

import "time"

// Source is a bounded, explicitly selected ROS topic subscription.
type Source struct {
	Key     string
	Topic   string
	Type    string
	Group   string
	Latched bool
	MaxAge  time.Duration
}

// SourceSnapshot is copied by the manager before a Prometheus collection.
// Data and its nested values must be treated as immutable after publication.
type SourceSnapshot struct {
	Spec          Source
	Publishers    int
	Subscribed    bool
	ReceivedAt    time.Time
	MessagesTotal uint64
	ErrorsTotal   uint64
	Data          map[string]any
}

// VehicleSnapshot contains the observations for exactly one configured vehicle.
type VehicleSnapshot struct {
	Name      string
	DomainID  int
	Connected bool
	StartedAt time.Time
	Sources   map[string]SourceSnapshot
}

// Fresh reports whether a usable message exists in the current ROS session.
// Notification topics have no age deadline because they may only change on events.
func (s SourceSnapshot) Fresh(now time.Time) bool {
	return s.Subscribed && s.Publishers > 0 && !s.ReceivedAt.IsZero() && s.Data != nil &&
		(s.Spec.Latched || s.Spec.MaxAge <= 0 || now.Sub(s.ReceivedAt) <= s.Spec.MaxAge)
}
