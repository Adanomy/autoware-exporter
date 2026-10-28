package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Adanomy/autoware-exporter/internal/exporter"
	"github.com/Adanomy/autoware-exporter/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

func TestHelpAndVersionWithoutROS(t *testing.T) {
	for _, flag := range []string{"--help", "--version"} {
		t.Run(flag, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := run([]string{flag}, &out, &errOut); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 || errOut.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
			}
		})
	}
}

func TestSelectedSources(t *testing.T) {
	sources, err := selectedSources([]string{"state", "performance"}, 3*time.Second)
	if err != nil || len(sources) == 0 {
		t.Fatalf("sources=%v err=%v", sources, err)
	}
	for _, source := range sources {
		if source.Group != "state" && source.Group != "performance" {
			t.Fatalf("unexpected group %s", source.Group)
		}
		if !source.Latched && source.MaxAge != 3*time.Second {
			t.Fatalf("periodic source TTL = %v", source.MaxAge)
		}
	}
	if _, err := selectedSources([]string{"typo"}, time.Second); err == nil || !strings.Contains(err.Error(), "typo") {
		t.Fatalf("unknown group accepted: %v", err)
	}
}

type fixedSnapshots []telemetry.VehicleSnapshot

func (s fixedSnapshots) Snapshots() []telemetry.VehicleSnapshot { return s }

func TestRoutingSourcesExportRouteEstimates(t *testing.T) {
	for _, groups := range [][]string{{"routing"}, {"state", "routing"}} {
		t.Run(strings.Join(groups, ","), func(t *testing.T) {
			sources, err := selectedSources(groups, 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			vehicle := telemetry.VehicleSnapshot{Name: "robot", Connected: true, Sources: make(map[string]telemetry.SourceSnapshot)}
			for _, source := range sources {
				if _, exists := vehicle.Sources[source.Key]; exists {
					t.Fatalf("duplicate source %q", source.Key)
				}
				if len(groups) == 1 && source.Group != "routing" && source.Key != "route_state" {
					t.Fatalf("unrelated source %q enabled by routing", source.Key)
				}
				var data map[string]any
				switch source.Key {
				case "route_state":
					data = map[string]any{"state": float64(2)}
				case "remaining_route":
					data = map[string]any{"remaining_distance": float64(125), "remaining_time": float64(30)}
				}
				vehicle.Sources[source.Key] = telemetry.SourceSnapshot{
					Spec: source, Subscribed: true, Publishers: 1, ReceivedAt: time.Now(), Data: data,
				}
			}
			registry := prometheus.NewPedanticRegistry()
			registry.MustRegister(exporter.NewCollector(fixedSnapshots{vehicle}, "test", "test"))
			families, err := registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]float64{"autoware_route_remaining_distance_meters": 125, "autoware_route_estimated_remaining_seconds": 30}
			for _, family := range families {
				if expected, ok := want[family.GetName()]; ok {
					if len(family.Metric) != 1 || family.Metric[0].GetGauge().GetValue() != expected {
						t.Errorf("%s = %v; want one sample with value %v", family.GetName(), family.Metric, expected)
					}
					delete(want, family.GetName())
				}
			}
			if len(want) != 0 {
				t.Fatalf("routing metrics missing: %v", want)
			}
		})
	}
}
