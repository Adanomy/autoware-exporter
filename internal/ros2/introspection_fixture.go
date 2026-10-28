//go:build linux && cgo && ros2 && ros2test

package ros2

/*
#include <stdlib.h>
#include <rosidl_runtime_c/primitives_sequence.h>
#include <rosidl_runtime_c/string.h>
#include <rosidl_typesupport_introspection_c/message_introspection.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Test fixtures use real installed std_msgs/sensor_msgs type support and its
// introspected offsets. This file is absent from production builds. CGO cannot
// be imported by a _test.go file, hence these small Go fixture setters.
func fixtureMember(m *messageType, name string) (*C.rosidl_typesupport_introspection_c__MessageMember, unsafe.Pointer, error) {
	for i := range int(m.members.member_count_) {
		member := &unsafe.Slice(m.members.members_, int(m.members.member_count_))[i]
		if C.GoString(member.name_) == name {
			return member, unsafe.Add(m.message, uintptr(member.offset_)), nil
		}
	}
	return nil, nil, fmt.Errorf("fixture field %s is missing", name)
}

func fixtureString(m *messageType, value string) error {
	_, pointer, err := fixtureMember(m, "data")
	if err != nil {
		return err
	}
	text := (*C.rosidl_runtime_c__String)(pointer)
	C.free(unsafe.Pointer(text.data))
	text.data = C.CString(value)
	text.size, text.capacity = C.size_t(len(value)), C.size_t(len(value)+1)
	return nil
}

func fixtureDouble(m *messageType, value float64) error {
	_, pointer, err := fixtureMember(m, "data")
	if err != nil {
		return err
	}
	*(*C.double)(pointer) = C.double(value)
	return nil
}

func fixtureBytes(m *messageType, values []byte) error {
	_, pointer, err := fixtureMember(m, "data")
	if err != nil {
		return err
	}
	sequence := (*C.rosidl_runtime_c__octet__Sequence)(pointer)
	C.free(unsafe.Pointer(sequence.data))
	sequence.data = nil
	sequence.size, sequence.capacity = 0, 0
	if len(values) != 0 {
		sequence.data = (*C.uint8_t)(C.calloc(C.size_t(len(values)), 1))
		if sequence.data == nil {
			return fmt.Errorf("allocate fixture sequence")
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(sequence.data)), len(values)), values)
		sequence.size, sequence.capacity = C.size_t(len(values)), C.size_t(len(values))
	}
	return nil
}

func fixtureFixedDoubles(m *messageType, name string, values []float64) error {
	member, pointer, err := fixtureMember(m, name)
	if err != nil {
		return err
	}
	if int(member.array_size_) != len(values) {
		return fmt.Errorf("fixture field %s has unexpected array length", name)
	}
	for i, value := range values {
		unsafe.Slice((*C.double)(pointer), len(values))[i] = C.double(value)
	}
	return nil
}
