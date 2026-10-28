package ros2

import "testing"

func TestValidateConfig(t *testing.T) {
	for _, c := range []Config{{DomainID: 51}, {DomainID: 0, Namespace: "/"}, {DomainID: 232, Namespace: "/vehicle_1/api"}} {
		if err := validateConfig(c); err != nil {
			t.Errorf("valid config %+v: %v", c, err)
		}
	}
	for _, c := range []Config{{DomainID: -1}, {DomainID: 233}, {Namespace: "relative"}, {Namespace: "/bad-name"}, {Namespace: "/a//b"}, {Namespace: "///"}, {Namespace: "/a//"}} {
		if err := validateConfig(c); err == nil {
			t.Errorf("accepted invalid config %+v", c)
		}
	}
}

func TestValidateSubscription(t *testing.T) {
	valid := Subscription{Key: "status", Topic: "/api/vehicle/status", Type: "autoware_adapi_v1_msgs/msg/VehicleStatus"}
	if err := validateSubscription(valid); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Subscription{
		{},
		{Key: "x", Topic: "/test", Type: "../std_msgs/msg/String"},
		{Key: "x", Topic: "/test", Type: "std_msgs/String"},
		{Key: "x", Topic: "/test", Type: "std_msgs/srv/Empty"},
		{Key: "x", Topic: "/test\x00suffix", Type: "std_msgs/msg/String"},
	} {
		if err := validateSubscription(s); err == nil {
			t.Errorf("accepted invalid subscription %+v", s)
		}
	}
}

func TestNodeName(t *testing.T) {
	if got := nodeName("vehicle-01/авто"); got != "autoware_exporter_vehicle_01_____" {
		t.Fatalf("node name = %q", got)
	}
}
