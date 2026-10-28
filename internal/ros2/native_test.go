//go:build linux && cgo && ros2

package ros2

import (
	"os"
	"testing"
	"time"
)

// This opt-in smoke test uses two otherwise unused DDS domains. It creates
// subscriptions only; no publishers, services, or vehicle commands are created.
func TestNativeClientLifecycle(t *testing.T) {
	if os.Getenv("AUTOWARE_EXPORTER_ROS_TESTS") != "1" {
		t.Skip("set AUTOWARE_EXPORTER_ROS_TESTS=1 in a sourced ROS installation")
	}
	clients := make([]*Client, 0, 2)
	// Leave room below the UDP port ceiling for middleware participant probes.
	for _, domain := range []int{201, 202} {
		client, err := New(Config{Name: "lifecycle", DomainID: domain})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
		// A missing message package must not poison this context.
		if err := client.Subscribe(Subscription{Key: "missing", Topic: "/autoware_exporter_test", Type: "missing_package/msg/Missing"}); err == nil {
			t.Fatal("missing type unexpectedly loaded")
		}
		// rcl validation failures must be copied/reset on the same OS thread
		// and leave the context usable for a subsequent valid subscription.
		if err := client.Subscribe(Subscription{Key: "invalid", Topic: "/invalid?topic", Type: "std_msgs/msg/String"}); err == nil {
			t.Fatal("invalid ROS topic unexpectedly accepted")
		}
		sub := Subscription{Key: "test", Topic: "/autoware_exporter_test", Type: "std_msgs/msg/String"}
		if err := client.Subscribe(sub); err != nil {
			t.Fatal(err)
		}
		if err := client.Subscribe(sub); err == nil {
			t.Fatal("duplicate key accepted")
		}
		if _, err := client.PublisherCount(sub.Topic); err != nil {
			t.Fatal(err)
		}
		if _, err := client.PublisherFingerprint(""); err == nil {
			t.Fatal("publisher query accepted an empty topic")
		}
		if fingerprint, err := client.PublisherFingerprint(sub.Topic); err != nil || fingerprint != "" {
			t.Fatalf("unpublished topic fingerprint = %q, error = %v", fingerprint, err)
		}
		if _, err := client.Poll(time.Millisecond); err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
	}
	if err := clients[0].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := clients[0].Poll(0); err == nil {
		t.Fatal("closed client accepted poll")
	}
	if _, err := clients[1].Poll(0); err != nil {
		t.Fatalf("closing the first DDS context affected the second: %v", err)
	}
}
