//go:build linux && cgo && ros2

package ros2

/*
#cgo linux LDFLAGS: -lrcl -lrcutils -lrmw -ldl
#include <stdlib.h>
#include <rcl/rcl.h>
#include <rcl/error_handling.h>
#include <rcl/graph.h>
#include <rmw/qos_profiles.h>
*/
import "C"

import (
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const (
	maxSubscriptions      = 256
	maxPublisherEndpoints = 256
	maxFingerprintBytes   = 4096
)

type nativeSubscription struct {
	key     string
	topic   string
	handle  *C.rcl_subscription_t
	message *messageType
}

type Client struct {
	mu            sync.Mutex
	context       *C.rcl_context_t
	node          *C.rcl_node_t
	waitset       *C.rcl_wait_set_t
	contextReady  bool
	nodeReady     bool
	waitsetReady  bool
	closed        bool
	subscriptions []*nativeSubscription
	keys          map[string]bool
}

// Generated C type-support getters initialize shared introspection tables
// lazily. Serialize setup across contexts; reception remains independent.
var setupMu sync.Mutex

func Available() bool { return true }

func New(config Config) (*Client, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	// ROS errors are thread-local. Keep each operation and its error read on
	// the same OS thread, including callers outside the per-vehicle worker.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	client := &Client{
		context: (*C.rcl_context_t)(C.calloc(1, C.sizeof_rcl_context_t)),
		node:    (*C.rcl_node_t)(C.calloc(1, C.sizeof_rcl_node_t)),
		waitset: (*C.rcl_wait_set_t)(C.calloc(1, C.sizeof_rcl_wait_set_t)),
		keys:    make(map[string]bool),
	}
	fail := func(err error) (*Client, error) {
		return nil, errors.Join(err, client.closeResources())
	}
	if client.context == nil || client.node == nil || client.waitset == nil {
		return fail(errors.New("allocate native ROS 2 context"))
	}
	*client.context = C.rcl_get_zero_initialized_context()
	*client.node = C.rcl_get_zero_initialized_node()
	*client.waitset = C.rcl_get_zero_initialized_wait_set()
	if err := client.initializeContext(config.DomainID); err != nil {
		return fail(err)
	}
	name := C.CString(nodeName(config.Name))
	defer C.free(unsafe.Pointer(name))
	ns := strings.TrimRight(config.Namespace, "/")
	if ns == "" {
		ns = "/"
	}
	namespace := C.CString(ns)
	defer C.free(unsafe.Pointer(namespace))
	options := C.rcl_node_get_default_options()
	options.enable_rosout = C.bool(false)
	// Keep use_global_arguments at its default. rcl_init received an empty
	// argument set; disabling it with unparsed local arguments is invalid.
	if C.rcl_node_init(client.node, name, namespace, client.context, &options) != C.RCL_RET_OK {
		return fail(nativeError("initialize ROS 2 node"))
	}
	client.nodeReady = true
	if C.rcl_wait_set_init(client.waitset, maxSubscriptions, 0, 0, 0, 0, 0,
		client.context, C.rcl_get_default_allocator()) != C.RCL_RET_OK {
		return fail(nativeError("initialize ROS wait set"))
	}
	client.waitsetReady = true
	return client, nil
}

func (c *Client) initializeContext(domain int) (err error) {
	// These options are only borrowed during calls. All rcl objects whose
	// addresses are retained by ROS are allocated in C memory.
	options := C.rcl_get_zero_initialized_init_options()
	if C.rcl_init_options_init(&options, C.rcl_get_default_allocator()) != C.RCL_RET_OK {
		return nativeError("initialize ROS context options")
	}
	defer func() {
		if C.rcl_init_options_fini(&options) != C.RCL_RET_OK {
			err = errors.Join(err, nativeError("finalize ROS context options"))
		}
	}()
	if C.rcl_init_options_set_domain_id(&options, C.size_t(domain)) != C.RCL_RET_OK {
		return nativeError("select ROS domain")
	}
	if C.rcl_init(0, nil, &options, c.context) != C.RCL_RET_OK {
		return nativeError("initialize ROS context")
	}
	c.contextReady = true
	return nil
}

func (c *Client) Subscribe(s Subscription) error {
	if err := validateSubscription(s); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("ROS 2 client is closed")
	}
	if c.keys[s.Key] {
		return fmt.Errorf("duplicate subscription key %q", s.Key)
	}
	if len(c.subscriptions) >= maxSubscriptions {
		return errors.New("maximum of 256 subscriptions per vehicle exceeded")
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	message, err := loadMessageType(s.Type)
	if err != nil {
		return fmt.Errorf("load message type %s: %w", s.Type, err)
	}
	handle := (*C.rcl_subscription_t)(C.calloc(1, C.sizeof_rcl_subscription_t))
	if handle == nil {
		message.close()
		return errors.New("allocate ROS subscription")
	}
	*handle = C.rcl_get_zero_initialized_subscription()
	topic := C.CString(s.Topic)
	defer C.free(unsafe.Pointer(topic))
	options := C.rcl_subscription_get_default_options()
	options.qos.history = C.RMW_QOS_POLICY_HISTORY_KEEP_LAST
	options.qos.depth = 10
	options.qos.reliability = C.RMW_QOS_POLICY_RELIABILITY_BEST_EFFORT
	options.qos.durability = C.RMW_QOS_POLICY_DURABILITY_VOLATILE
	if s.Latched {
		options.qos.depth = 1
		options.qos.reliability = C.RMW_QOS_POLICY_RELIABILITY_RELIABLE
		options.qos.durability = C.RMW_QOS_POLICY_DURABILITY_TRANSIENT_LOCAL
	}
	if C.rcl_subscription_init(handle, c.node, message.support, topic, &options) != C.RCL_RET_OK {
		err := nativeError("subscribe " + s.Topic + " (" + s.Type + ")")
		C.free(unsafe.Pointer(handle))
		message.close()
		return err
	}
	c.subscriptions = append(c.subscriptions, &nativeSubscription{
		key: s.Key, topic: s.Topic, handle: handle, message: message,
	})
	c.keys[s.Key] = true
	return nil
}

// Poll waits at most timeout and drains at most ten messages per subscription.
// A rejected source message does not interrupt the other subscriptions.
func (c *Client) Poll(timeout time.Duration) ([]Message, error) {
	if timeout < 0 || timeout > time.Second {
		return nil, errors.New("ROS poll timeout must be between zero and one second")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("ROS 2 client is closed")
	}
	if len(c.subscriptions) == 0 {
		time.Sleep(timeout)
		return nil, nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if C.rcl_wait_set_clear(c.waitset) != C.RCL_RET_OK {
		return nil, nativeError("clear ROS wait set")
	}
	for _, subscription := range c.subscriptions {
		if C.rcl_wait_set_add_subscription(c.waitset, subscription.handle, nil) != C.RCL_RET_OK {
			return nil, nativeError("add ROS subscription to wait set")
		}
	}
	if result := C.rcl_wait(c.waitset, C.int64_t(timeout)); result != C.RCL_RET_OK {
		if result == C.RCL_RET_TIMEOUT {
			C.rcl_reset_error()
			return nil, nil
		}
		return nil, nativeError("wait for ROS messages")
	}
	var messages []Message
	for _, subscription := range c.subscriptions {
		for range 10 {
			var info C.rmw_message_info_t
			result := C.rcl_take(subscription.handle, subscription.message.message, &info, nil)
			if result == C.RCL_RET_SUBSCRIPTION_TAKE_FAILED {
				C.rcl_reset_error()
				break
			}
			if result != C.RCL_RET_OK {
				messages = append(messages, Message{Key: subscription.key, Error: nativeError("take " + subscription.topic)})
				break
			}
			payload, err := subscription.message.read()
			if err != nil {
				subscription.message.reset()
				messages = append(messages, Message{Key: subscription.key, Error: fmt.Errorf("read %s: %w", subscription.topic, err)})
				break
			}
			messages = append(messages, Message{Key: subscription.key, Data: payload})
		}
	}
	return messages, nil
}

func (c *Client) PublisherCount(topic string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, errors.New("ROS 2 client is closed")
	}
	if err := validatePublisherTopic(topic); err != nil {
		return 0, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name := C.CString(topic)
	defer C.free(unsafe.Pointer(name))
	var count C.size_t
	if C.rcl_count_publishers(c.node, name, &count) != C.RCL_RET_OK {
		return 0, nativeError("count publishers " + topic)
	}
	return int(count), nil
}

// PublisherFingerprint returns sorted endpoint GIDs. Unlike publisher counts,
// this changes when a publisher is replaced between graph checks.
func (c *Client) PublisherFingerprint(topic string) (fingerprint string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", errors.New("ROS 2 client is closed")
	}
	if err := validatePublisherTopic(topic); err != nil {
		return "", err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name := C.CString(topic)
	defer C.free(unsafe.Pointer(name))
	allocator := C.rcl_get_default_allocator()
	endpoints := C.rcl_get_zero_initialized_topic_endpoint_info_array()
	defer func() {
		if C.rcl_topic_endpoint_info_array_fini(&endpoints, &allocator) != C.RMW_RET_OK {
			fingerprint = ""
			err = errors.Join(err, nativeError("finalize publisher endpoint information"))
		}
	}()
	if C.rcl_get_publishers_info_by_topic(c.node, &allocator, name, false, &endpoints) != C.RCL_RET_OK {
		return "", nativeError("inspect publishers " + topic)
	}
	count := int(endpoints.size)
	if endpoints.size > maxPublisherEndpoints || count > maxFingerprintBytes/(2*int(C.RMW_GID_STORAGE_SIZE)+1) {
		return "", errors.New("publisher fingerprint exceeds 256 endpoints or 4 KiB")
	}
	gids := make([]string, 0, count)
	for _, endpoint := range unsafe.Slice(endpoints.info_array, count) {
		gid := unsafe.Slice((*byte)(unsafe.Pointer(&endpoint.endpoint_gid[0])), int(C.RMW_GID_STORAGE_SIZE))
		gids = append(gids, hex.EncodeToString(gid))
	}
	slices.Sort(gids)
	return strings.Join(gids, ","), nil
}

func validatePublisherTopic(topic string) error {
	if topic == "" || strings.ContainsRune(topic, '\x00') {
		return errors.New("publisher topic must be nonempty and contain no NUL bytes")
	}
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return c.closeResources()
}

// closeResources is called with setupMu held and the current OS thread locked.
// New also uses it to unwind a partially initialized context.
func (c *Client) closeResources() error {
	var failures []error
	if c.waitsetReady {
		if C.rcl_wait_set_fini(c.waitset) != C.RCL_RET_OK {
			failures = append(failures, nativeError("finalize ROS wait set"))
		}
	}
	for _, subscription := range c.subscriptions {
		if C.rcl_subscription_fini(subscription.handle, c.node) != C.RCL_RET_OK {
			failures = append(failures, nativeError("finalize ROS subscription"))
		}
		subscription.message.close()
		C.free(unsafe.Pointer(subscription.handle))
	}
	if c.nodeReady {
		if C.rcl_node_fini(c.node) != C.RCL_RET_OK {
			failures = append(failures, nativeError("finalize ROS node"))
		}
	}
	if c.contextReady {
		if C.rcl_shutdown(c.context) != C.RCL_RET_OK {
			failures = append(failures, nativeError("shut down ROS context"))
		}
		if C.rcl_context_fini(c.context) != C.RCL_RET_OK {
			failures = append(failures, nativeError("finalize ROS context"))
		}
	}
	C.free(unsafe.Pointer(c.waitset))
	C.free(unsafe.Pointer(c.node))
	C.free(unsafe.Pointer(c.context))
	c.waitset, c.node, c.context = nil, nil, nil
	c.subscriptions = nil
	c.keys = nil
	c.closed = true
	return errors.Join(failures...)
}

// ROS stores its error on the calling thread. Call this immediately after the
// failing native operation, before another rcl call can replace the error.
func nativeError(operation string) error {
	diagnostic := C.rcl_get_error_string()
	message := C.GoString(&diagnostic.str[0])
	C.rcl_reset_error()
	if message == "" {
		message = "native operation failed"
	}
	return fmt.Errorf("%s: %s", operation, message)
}
