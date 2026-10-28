// Package config parses and validates command-line configuration.
package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Vehicle identifies one independent ROS graph by domain and namespace.
type Vehicle struct {
	Name      string
	DomainID  int
	Namespace string
}

// Config contains the exporter configuration. Empty Collectors enables all groups.
type Config struct {
	ListenAddress string
	Vehicles      []Vehicle
	Collectors    []string
	MaxAge        time.Duration
	PollInterval  time.Duration
	RetryInterval time.Duration
	LogLevel      string
	LogFormat     string
	Version       bool
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	if value == "" {
		return fmt.Errorf("vehicle must not be empty")
	}
	*s = append(*s, value)
	return nil
}

var (
	vehicleName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	rosToken    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func flags(c *Config, vehicles *stringList, collectors *string, output io.Writer) *flag.FlagSet {
	f := flag.NewFlagSet("autoware-exporter", flag.ContinueOnError)
	f.SetOutput(output)
	f.StringVar(&c.ListenAddress, "web.listen-address", ":9841", "HTTP listen address.")
	f.Var(vehicles, "vehicle", "Vehicle as name:domain[:namespace]; repeat for multiple vehicles. Default: default:0.")
	f.StringVar(collectors, "collectors", "", "Comma-separated collector groups; empty enables all groups.")
	f.DurationVar(&c.MaxAge, "source.max-age", 10*time.Second, "Maximum receive age of periodic sources; state notifications do not expire by age.")
	f.DurationVar(&c.PollInterval, "ros.poll-interval", 50*time.Millisecond, "Maximum wait per ROS receive iteration.")
	f.DurationVar(&c.RetryInterval, "ros.retry-interval", 5*time.Second, "Retry interval for unavailable ROS clients and subscriptions.")
	f.StringVar(&c.LogLevel, "log.level", "info", "Log level: debug, info, warn, or error.")
	f.StringVar(&c.LogFormat, "log.format", "text", "Log format: text or json.")
	f.BoolVar(&c.Version, "version", false, "Print build information and exit.")
	return f
}

// Usage writes command-line help without parsing or changing configuration.
func Usage(output io.Writer) {
	var c Config
	var vehicles stringList
	var collectors string
	f := flags(&c, &vehicles, &collectors, output)
	f.Usage()
}

// Parse returns validated configuration. Help requests return flag.ErrHelp.
func Parse(args []string) (Config, error) {
	var c Config
	var vehicles stringList
	var collectors string
	f := flags(&c, &vehicles, &collectors, io.Discard)
	if err := f.Parse(args); err != nil {
		return c, err
	}
	if f.NArg() != 0 {
		return c, fmt.Errorf("unexpected positional arguments: %s", strings.Join(f.Args(), " "))
	}
	if c.Version {
		return c, nil
	}
	if err := validateAddress(c.ListenAddress); err != nil {
		return c, err
	}
	if c.MaxAge <= 0 || c.MaxAge > time.Hour {
		return c, fmt.Errorf("source.max-age must be greater than zero and at most 1h")
	}
	if c.PollInterval < time.Millisecond || c.PollInterval > time.Second {
		return c, fmt.Errorf("ros.poll-interval must be between 1ms and 1s")
	}
	if c.RetryInterval < 100*time.Millisecond || c.RetryInterval > 5*time.Minute {
		return c, fmt.Errorf("ros.retry-interval must be between 100ms and 5m")
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, c.LogLevel) {
		return c, fmt.Errorf("invalid log.level %q", c.LogLevel)
	}
	if !slices.Contains([]string{"text", "json"}, c.LogFormat) {
		return c, fmt.Errorf("invalid log.format %q", c.LogFormat)
	}
	if len(vehicles) == 0 {
		vehicles = []string{"default:0"}
	}
	names, targets := map[string]bool{}, map[string]bool{}
	for _, value := range vehicles {
		v, err := parseVehicle(value)
		if err != nil {
			return c, fmt.Errorf("invalid vehicle %q: %w", value, err)
		}
		if names[v.Name] {
			return c, fmt.Errorf("duplicate vehicle name %q", v.Name)
		}
		target := strconv.Itoa(v.DomainID) + ":" + v.Namespace
		if targets[target] {
			return c, fmt.Errorf("duplicate vehicle target domain=%d namespace=%q", v.DomainID, v.Namespace)
		}
		names[v.Name], targets[target] = true, true
		c.Vehicles = append(c.Vehicles, v)
	}
	if collectors != "" {
		for name := range strings.SplitSeq(collectors, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				return c, fmt.Errorf("collectors must not contain an empty group")
			}
			if !slices.Contains(c.Collectors, name) {
				c.Collectors = append(c.Collectors, name)
			}
		}
	}
	return c, nil
}

func validateAddress(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid web.listen-address: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid web.listen-address: expected port in range 1..65535")
	}
	return nil
}

func parseVehicle(value string) (Vehicle, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return Vehicle{}, fmt.Errorf("expected name:domain[:namespace]")
	}
	v := Vehicle{Name: parts[0], Namespace: "/"}
	if !vehicleName.MatchString(v.Name) {
		return v, fmt.Errorf("name must contain 1..64 letters, digits, dots, underscores or hyphens and start with a letter or digit")
	}
	domain, err := strconv.Atoi(parts[1])
	if err != nil || domain < 0 || domain > 232 {
		return v, fmt.Errorf("domain must be an integer in range 0..232")
	}
	v.DomainID = domain
	if len(parts) == 3 && parts[2] != "" && parts[2] != "/" {
		namespace := strings.TrimSuffix(strings.TrimPrefix(parts[2], "/"), "/")
		for token := range strings.SplitSeq(namespace, "/") {
			if !rosToken.MatchString(token) {
				return v, fmt.Errorf("namespace must contain ROS name tokens separated by single slashes")
			}
		}
		v.Namespace = "/" + namespace
	}
	return v, nil
}
