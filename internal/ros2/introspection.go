//go:build linux && cgo && ros2

package ros2

/*
#include <dlfcn.h>
#include <float.h>
#include <stdlib.h>
#include <string.h>
#include <rosidl_runtime_c/primitives_sequence.h>
#include <rosidl_runtime_c/string.h>
#include <rosidl_runtime_c/u16string.h>
#include <rosidl_typesupport_introspection_c/field_types.h>
#include <rosidl_typesupport_introspection_c/message_introspection.h>

// CGO cannot invoke function pointers directly. These wrappers only invoke
// generated ROS callbacks; ownership and traversal stay in Go.
static inline const rosidl_message_type_support_t *ae_type_support(void *symbol) {
  return ((const rosidl_message_type_support_t *(*)(void))symbol)();
}
static inline void ae_message_init(const rosidl_typesupport_introspection_c__MessageMembers *members, void *message) {
  members->init_function(message, ROSIDL_RUNTIME_C_MSG_INIT_ALL);
}
static inline void ae_message_fini(const rosidl_typesupport_introspection_c__MessageMembers *members, void *message) {
  members->fini_function(message);
}
static inline size_t ae_array_size(const rosidl_typesupport_introspection_c__MessageMember *member, const void *array) {
  return member->size_function(array);
}
static inline const void *ae_array_get(const rosidl_typesupport_introspection_c__MessageMember *member, const void *array, size_t index) {
  return member->get_const_function(array, index);
}
enum { ae_long_double_size = sizeof(long double) };
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

// Every ABI-sensitive field comes from installed ROS headers. Generated type
// support libraries own the metadata; both handles stay open until after fini.
type messageType struct {
	support              *C.rosidl_message_type_support_t
	members              *C.rosidl_typesupport_introspection_c__MessageMembers
	message              unsafe.Pointer
	typesupportLibrary   unsafe.Pointer
	introspectionLibrary unsafe.Pointer
	initialized          bool
}

func loadMessageType(typeName string) (_ *messageType, err error) {
	if !messageTypePattern.MatchString(typeName) || len(typeName) > 512 {
		return nil, fmt.Errorf("invalid ROS message type %q", typeName)
	}
	// The dynamic loader's diagnostic is thread-local. Copy it on the same
	// thread as the operation; callers serialize generated lazy getters.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	parts := strings.Split(typeName, "/")
	m := &messageType{}
	defer func() {
		if err != nil {
			m.close()
		}
	}()
	m.typesupportLibrary, err = openLibrary("lib" + parts[0] + "__rosidl_typesupport_c.so")
	if err != nil {
		return nil, err
	}
	getter, err := librarySymbol(m.typesupportLibrary, "rosidl_typesupport_c__get_message_type_support_handle__"+parts[0]+"__msg__"+parts[2])
	if err != nil {
		return nil, err
	}
	m.support = C.ae_type_support(getter)
	m.introspectionLibrary, err = openLibrary("lib" + parts[0] + "__rosidl_typesupport_introspection_c.so")
	if err != nil {
		return nil, err
	}
	getter, err = librarySymbol(m.introspectionLibrary, "rosidl_typesupport_introspection_c__get_message_type_support_handle__"+parts[0]+"__msg__"+parts[2])
	if err != nil {
		return nil, err
	}
	introspection := C.ae_type_support(getter)
	if m.support == nil || introspection == nil || introspection.data == nil {
		return nil, errors.New("ROS message type support is empty")
	}
	m.members = (*C.rosidl_typesupport_introspection_c__MessageMembers)(unsafe.Pointer(introspection.data))
	if m.members.size_of_ == 0 || uint64(m.members.size_of_) > maxDecodedBytes || m.members.init_function == nil || m.members.fini_function == nil {
		return nil, errors.New("ROS introspection has missing lifecycle functions or exceeds the 4 MiB message allocation limit")
	}
	m.message = C.calloc(1, m.members.size_of_)
	if m.message == nil {
		return nil, errors.New("allocate ROS message")
	}
	C.ae_message_init(m.members, m.message)
	m.initialized = true
	return m, nil
}

func openLibrary(name string) (unsafe.Pointer, error) {
	text := C.CString(name)
	defer C.free(unsafe.Pointer(text))
	handle := C.dlopen(text, C.RTLD_NOW|C.RTLD_LOCAL)
	if handle == nil {
		return nil, fmt.Errorf("load %s: %s", name, loaderDiagnostic())
	}
	return handle, nil
}

func librarySymbol(library unsafe.Pointer, name string) (unsafe.Pointer, error) {
	text := C.CString(name)
	defer C.free(unsafe.Pointer(text))
	C.dlerror()
	symbol := C.dlsym(library, text)
	if symbol == nil {
		return nil, fmt.Errorf("load symbol %s: %s", name, loaderDiagnostic())
	}
	return symbol, nil
}

func loaderDiagnostic() string {
	message := C.dlerror()
	if message == nil {
		return "dynamic loader returned no diagnostic"
	}
	return C.GoString(message)
}

func (m *messageType) reset() {
	if m.message == nil || !m.initialized {
		return
	}
	C.ae_message_fini(m.members, m.message)
	C.memset(m.message, 0, m.members.size_of_)
	C.ae_message_init(m.members, m.message)
}

func (m *messageType) close() {
	if m == nil {
		return
	}
	if m.message != nil {
		if m.initialized {
			C.ae_message_fini(m.members, m.message)
		}
		C.free(m.message)
		m.message = nil
	}
	m.initialized = false
	m.support = nil
	m.members = nil
	if m.introspectionLibrary != nil {
		C.dlclose(m.introspectionLibrary)
		m.introspectionLibrary = nil
	}
	if m.typesupportLibrary != nil {
		C.dlclose(m.typesupportLibrary)
		m.typesupportLibrary = nil
	}
}

// read deep-copies a taken message into Go values. No pointer into the ROS
// allocation or its nested strings/sequences is retained in the result.
func (m *messageType) read() (map[string]any, error) {
	if m == nil || m.members == nil || m.message == nil {
		return nil, errors.New("ROS message type is closed")
	}
	budget := decodeBudget{}
	return readMembers(m.members, m.message, 0, &budget)
}

func readMembers(members *C.rosidl_typesupport_introspection_c__MessageMembers, message unsafe.Pointer, depth int, budget *decodeBudget) (map[string]any, error) {
	if err := checkMessageDepth(depth); err != nil {
		return nil, err
	}
	if members == nil || message == nil || members.size_of_ == 0 || members.members_ == nil && members.member_count_ != 0 {
		return nil, errors.New("ROS message introspection is missing")
	}
	if err := checkArraySize(uint64(members.member_count_)); err != nil {
		return nil, err
	}
	count := int(members.member_count_)
	if err := budget.take(64+count*32, 1); err != nil {
		return nil, err
	}
	result := make(map[string]any, count)
	for i := range count {
		member := &unsafe.Slice(members.members_, count)[i]
		if member.name_ == nil {
			return nil, errors.New("ROS introspection field has no name")
		}
		nameSize := int(C.strnlen(member.name_, maxFieldNameBytes+1))
		if nameSize == 0 || nameSize > maxFieldNameBytes {
			return nil, errors.New("ROS introspection field name is empty or exceeds 512 bytes")
		}
		if err := budget.take(nameSize, 0); err != nil {
			return nil, err
		}
		name := C.GoStringN(member.name_, C.int(nameSize))
		if !utf8.ValidString(name) {
			return nil, errors.New("ROS introspection field name is not valid UTF-8")
		}
		offset, width := uint64(member.offset_), uint64(scalarSize(member))
		if width == 0 {
			return nil, fmt.Errorf("field %s: unsupported ROS field type %d", name, member.type_id_)
		}
		if bool(member.is_array_) {
			if member.array_size_ != 0 && !bool(member.is_upper_bound_) {
				if err := checkArraySize(uint64(member.array_size_)); err != nil {
					return nil, fmt.Errorf("field %s: %w", name, err)
				}
				if width > uint64(members.size_of_)/uint64(member.array_size_) {
					return nil, fmt.Errorf("field %s: fixed array size exceeds message allocation", name)
				}
				width *= uint64(member.array_size_)
			} else {
				width = uint64(C.sizeof_rosidl_runtime_c__octet__Sequence)
			}
		}
		if offset > uint64(members.size_of_) || width > uint64(members.size_of_)-offset {
			return nil, fmt.Errorf("field %s: introspection offset exceeds message allocation", name)
		}
		pointer := unsafe.Add(message, uintptr(offset))
		var value any
		var err error
		if bool(member.is_array_) {
			value, err = readArray(member, pointer, depth, budget)
		} else {
			value, err = readScalar(member, pointer, depth, budget)
		}
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", name, err)
		}
		result[name] = value
	}
	return result, nil
}

func scalarSize(member *C.rosidl_typesupport_introspection_c__MessageMember) uintptr {
	switch member.type_id_ {
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_FLOAT:
		return C.sizeof_float
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_DOUBLE:
		return C.sizeof_double
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_LONG_DOUBLE:
		return C.ae_long_double_size
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_CHAR, C.rosidl_typesupport_introspection_c__ROS_TYPE_OCTET, C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT8, C.rosidl_typesupport_introspection_c__ROS_TYPE_INT8:
		return 1
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_WCHAR, C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT16, C.rosidl_typesupport_introspection_c__ROS_TYPE_INT16:
		return 2
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT32, C.rosidl_typesupport_introspection_c__ROS_TYPE_INT32:
		return 4
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT64, C.rosidl_typesupport_introspection_c__ROS_TYPE_INT64:
		return 8
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_BOOLEAN:
		return C.sizeof_bool
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_STRING:
		return C.sizeof_rosidl_runtime_c__String
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_WSTRING:
		return C.sizeof_rosidl_runtime_c__U16String
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_MESSAGE:
		if member.members_ != nil && member.members_.data != nil {
			return uintptr((*C.rosidl_typesupport_introspection_c__MessageMembers)(unsafe.Pointer(member.members_.data)).size_of_)
		}
	}
	return 0
}

func readArray(member *C.rosidl_typesupport_introspection_c__MessageMember, pointer unsafe.Pointer, depth int, budget *decodeBudget) ([]any, error) {
	count := uint64(member.array_size_)
	data := pointer
	if member.array_size_ == 0 || bool(member.is_upper_bound_) {
		// Generated ROS C sequences share the public data/size/capacity layout;
		// the fields here are obtained from the installed primitive sequence ABI.
		sequence := (*C.rosidl_runtime_c__octet__Sequence)(pointer)
		count = uint64(sequence.size)
		data = unsafe.Pointer(sequence.data)
		if sequence.size > sequence.capacity {
			return nil, errors.New("ROS sequence size exceeds its capacity")
		}
		if member.size_function != nil {
			count = uint64(C.ae_array_size(member, pointer))
		}
		if count > uint64(sequence.capacity) {
			return nil, errors.New("ROS sequence callback size exceeds its capacity")
		}
		if bool(member.is_upper_bound_) && member.array_size_ != 0 && count > uint64(member.array_size_) {
			return nil, errors.New("ROS sequence exceeds its declared upper bound")
		}
	}
	if err := checkArraySize(count); err != nil {
		return nil, err
	}
	if count > 0 && data == nil && member.get_const_function == nil {
		return nil, errors.New("ROS array data is missing")
	}
	if err := budget.take(24+int(count)*16, 1); err != nil {
		return nil, err
	}
	result := make([]any, int(count))
	stride := scalarSize(member)
	if stride == 0 {
		return nil, errors.New("ROS array element type is unsupported")
	}
	for i := range result {
		var item unsafe.Pointer
		if member.get_const_function != nil {
			item = unsafe.Pointer(C.ae_array_get(member, pointer, C.size_t(i)))
		} else {
			item = unsafe.Add(data, uintptr(i)*stride)
		}
		if item == nil {
			return nil, errors.New("ROS array element is missing")
		}
		value, err := readScalar(member, item, depth, budget)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		result[i] = value
	}
	return result, nil
}

func readScalar(member *C.rosidl_typesupport_introspection_c__MessageMember, pointer unsafe.Pointer, depth int, budget *decodeBudget) (any, error) {
	if member.type_id_ == C.rosidl_typesupport_introspection_c__ROS_TYPE_MESSAGE {
		if member.members_ == nil || member.members_.data == nil {
			return nil, errors.New("nested ROS introspection is missing")
		}
		return readMembers((*C.rosidl_typesupport_introspection_c__MessageMembers)(unsafe.Pointer(member.members_.data)), pointer, depth+1, budget)
	}
	if err := budget.take(16, 1); err != nil {
		return nil, err
	}
	var value float64
	switch member.type_id_ {
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_FLOAT:
		value = float64(*(*C.float)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_DOUBLE:
		value = float64(*(*C.double)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_LONG_DOUBLE:
		var err error
		value, err = readLongDouble(pointer)
		if err != nil {
			return nil, err
		}
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_BOOLEAN:
		return bool(*(*C.bool)(pointer)), nil
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_CHAR, C.rosidl_typesupport_introspection_c__ROS_TYPE_OCTET, C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT8:
		value = float64(*(*C.uint8_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_INT8:
		value = float64(*(*C.int8_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_WCHAR, C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT16:
		value = float64(*(*C.uint16_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_INT16:
		value = float64(*(*C.int16_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT32:
		value = float64(*(*C.uint32_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_INT32:
		value = float64(*(*C.int32_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_UINT64:
		value = float64(*(*C.uint64_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_INT64:
		value = float64(*(*C.int64_t)(pointer))
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_STRING:
		text := (*C.rosidl_runtime_c__String)(pointer)
		if text.size > maxDecodedBytes || text.size > 0 && (text.data == nil || text.size >= text.capacity) {
			return nil, errors.New("ROS string has invalid size/capacity or exceeds 4 MiB")
		}
		if member.string_upper_bound_ != 0 && text.size > member.string_upper_bound_ {
			return nil, errors.New("ROS string exceeds its declared upper bound")
		}
		if err := budget.take(int(text.size), 0); err != nil {
			return nil, err
		}
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(text.data)), int(text.size))
		if !utf8.Valid(bytes) {
			return nil, errors.New("ROS string is not valid UTF-8")
		}
		return string(bytes), nil
	case C.rosidl_typesupport_introspection_c__ROS_TYPE_WSTRING:
		text := (*C.rosidl_runtime_c__U16String)(pointer)
		if text.size > maxDecodedBytes/2 || text.size > 0 && (text.data == nil || text.size >= text.capacity) {
			return nil, errors.New("ROS UTF-16 string has invalid size/capacity or exceeds 4 MiB")
		}
		if member.string_upper_bound_ != 0 && text.size > member.string_upper_bound_ {
			return nil, errors.New("ROS UTF-16 string exceeds its declared upper bound")
		}
		units := unsafe.Slice((*uint16)(unsafe.Pointer(text.data)), int(text.size))
		if err := budget.take(utf16ByteLength(units), 0); err != nil {
			return nil, err
		}
		return string(utf16.Decode(units)), nil
	default:
		return nil, fmt.Errorf("unsupported ROS field type %d", member.type_id_)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, nil
	}
	return value, nil
}

// ROS exposes long double in introspection although the configured metrics use
// ordinary float/double. Decode supported Linux representations in Go because
// CGO cannot represent a C long double in a Go value.
func readLongDouble(pointer unsafe.Pointer) (float64, error) {
	bytes := unsafe.Slice((*byte)(pointer), int(C.ae_long_double_size))
	if C.LDBL_MANT_DIG == 53 && len(bytes) == 8 {
		return *(*float64)(pointer), nil
	}
	if C.LDBL_MANT_DIG == 64 && runtime.GOARCH == "amd64" && len(bytes) >= 10 {
		signExponent := binary.LittleEndian.Uint16(bytes[8:10])
		exponent := int(signExponent & 0x7fff)
		if exponent == 0x7fff {
			return math.NaN(), nil
		}
		if exponent == 0 {
			exponent = 1
		}
		value := math.Ldexp(float64(binary.LittleEndian.Uint64(bytes[:8])), exponent-16383-63)
		if signExponent&0x8000 != 0 {
			value = -value
		}
		return value, nil
	}
	if C.LDBL_MANT_DIG == 113 && runtime.GOARCH == "arm64" && len(bytes) == 16 {
		low, high := binary.LittleEndian.Uint64(bytes[:8]), binary.LittleEndian.Uint64(bytes[8:])
		exponent := int((high >> 48) & 0x7fff)
		if exponent == 0x7fff {
			return math.NaN(), nil
		}
		mantissa := new(big.Int).SetUint64(high & 0x0000ffffffffffff)
		mantissa.Lsh(mantissa, 64).Or(mantissa, new(big.Int).SetUint64(low))
		if exponent == 0 {
			exponent = 1
		} else {
			mantissa.SetBit(mantissa, 112, 1)
		}
		value := new(big.Float).SetPrec(113).SetInt(mantissa)
		value.SetMantExp(value, exponent-16383-112)
		result, _ := value.Float64()
		if high>>63 != 0 {
			result = -result
		}
		return result, nil
	}
	return 0, errors.New("unsupported native long double representation")
}
