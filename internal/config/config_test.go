package config

import (
	"bytes"
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddress != ":9841" || c.MaxAge != 10*time.Second || c.PollInterval != 50*time.Millisecond || c.RetryInterval != 5*time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if !reflect.DeepEqual(c.Vehicles, []Vehicle{{Name: "default", DomainID: 0, Namespace: "/"}}) {
		t.Fatalf("unexpected vehicles: %+v", c.Vehicles)
	}
}

func TestVehiclesAndCollectors(t *testing.T) {
	c, err := Parse([]string{"--vehicle=car-01:51", "--vehicle=car-02:52:/fleet/car_02/", "--vehicle=car-03:52:car_03", "--collectors=state, vehicle,state", "--log.level=debug", "--log.format=json", "--source.max-age=30s"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Vehicle{{Name: "car-01", DomainID: 51, Namespace: "/"}, {Name: "car-02", DomainID: 52, Namespace: "/fleet/car_02"}, {Name: "car-03", DomainID: 52, Namespace: "/car_03"}}
	if !reflect.DeepEqual(c.Vehicles, want) || !reflect.DeepEqual(c.Collectors, []string{"state", "vehicle"}) || c.MaxAge != 30*time.Second || c.LogLevel != "debug" || c.LogFormat != "json" {
		t.Fatalf("unexpected configuration: %+v", c)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	cases := [][]string{
		{"--vehicle="}, {"--vehicle=car"}, {"--vehicle=:51"}, {"--vehicle=car:abc"}, {"--vehicle=car:-1"}, {"--vehicle=car:233"},
		{"--vehicle=car:51:a:b"}, {"--vehicle=car:51:/a//b"}, {"--vehicle=car:51:/a/1b"}, {"--vehicle=car:51:/a-b"}, {"--vehicle=car name:51"},
		{"--vehicle=a:51", "--vehicle=a:52"}, {"--vehicle=a:51", "--vehicle=b:51:/"}, {"--vehicle=a:51:foo", "--vehicle=b:51:/foo/"},
		{"--web.listen-address=localhost"}, {"--web.listen-address=:0"}, {"--web.listen-address=:65536"}, {"--web.listen-address=:http"},
		{"--source.max-age=0"}, {"--source.max-age=-1s"}, {"--source.max-age=2h"}, {"--ros.poll-interval=0"}, {"--ros.poll-interval=2s"},
		{"--ros.retry-interval=1ms"}, {"--ros.retry-interval=6m"}, {"--log.level=trace"}, {"--log.format=yaml"}, {"--collectors=state,,vehicle"},
		{"--collectors= "}, {"unexpected"}, {"--unknown"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := Parse(args); err == nil {
				t.Fatalf("accepted invalid arguments: %q", args)
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	if _, err := Parse([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help error: %v", err)
	}
	c, err := Parse([]string{"--version", "--vehicle=not-parsed"})
	if err != nil || !c.Version {
		t.Fatalf("version should bypass runtime validation: %+v, %v", c, err)
	}
	var output bytes.Buffer
	Usage(&output)
	if !strings.Contains(output.String(), "-vehicle") || !strings.Contains(output.String(), "-web.listen-address") {
		t.Fatalf("incomplete usage: %s", output.String())
	}
}
