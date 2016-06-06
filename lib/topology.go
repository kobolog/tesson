package tesson

// #include <hwloc.h>
// #cgo LDFLAGS: -lhwloc
import "C"

import (
	"errors"
)

var (
	errInternalHwlocError = errors.New("internal hwloc error")
)

// Topology represents the machine's hardware topology.
type Topology interface {
	N() int
	Distribute(n int, opts DistributeOptions) ([]string, error)
}

// DistributeOptions specifies options for Distribute.
type DistributeOptions struct {
	Granularity Granularity
}

// DefaultDistribution returns default options for Distribute.
func DefaultDistribution() DistributeOptions {
	return DistributeOptions{Granularity: Core}
}

// Granularity specifies distribution granularity.
type Granularity uint

// Supported distribution granularities.
const (
	Node Granularity = iota
	Core
)

// Implementation

// NewHwlocTopology returns a Topology object for this machine,
// implemented in terms of libhwloc.
func NewHwlocTopology() (Topology, error) {
	t := &hwloc{}

	var r C.int

	r = C.hwloc_topology_init(&t.ptr)

	if r != 0 {
		return nil, errInternalHwlocError
	}

	r = C.hwloc_topology_load(t.ptr)

	if r != 0 {
		return nil, errInternalHwlocError
	}

	return t, nil
}

type hwloc struct {
	ptr C.hwloc_topology_t
}

func (g Granularity) build() C.hwloc_obj_type_t {
	switch g {
	case Node:
		return C.HWLOC_OBJ_NODE
	case Core:
		return C.HWLOC_OBJ_CORE
	}

	panic("topology: unknown object type")
}

func (t *hwloc) N() int {
	return int(C.hwloc_get_nbobjs_by_type(t.ptr, C.HWLOC_OBJ_CORE))
}

func (t *hwloc) Distribute(
	n int, opts DistributeOptions) ([]string, error) {

	var (
		roots = C.hwloc_get_root_obj(t.ptr)
		depth = C.hwloc_get_type_or_below_depth(
			t.ptr, opts.Granularity.build())
		l = make([]C.hwloc_cpuset_t, n)
	)

	C.hwloc_distrib(
		t.ptr, &roots, 1, &l[0], C.uint(len(l)), C.uint(depth), 0)

	r := make([]string, n)
	b := [64]C.char{}

	for i, c := range l {
		n := C.hwloc_bitmap_list_snprintf(&b[0], 64, c)
		r[i] = C.GoStringN(&b[0], n)
	}

	return r, nil
}
