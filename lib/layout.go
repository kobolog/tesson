/*
   Copyright (c) 2016 Andrey Sibiryov <me@kobology.ru>
   Copyright (c) 2016 Other contributors as noted in the AUTHORS file.

   This file is part of Tesson.

   Tesson is free software; you can redistribute it and/or modify
   it under the terms of the GNU Lesser General Public License as published by
   the Free Software Foundation; either version 3 of the License, or
   (at your option) any later version.

   Tesson is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
   GNU Lesser General Public License for more details.

   You should have received a copy of the GNU Lesser General Public License
   along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

package tesson

// #include <hwloc.h>
// #cgo CFLAGS: -Wno-deprecated-declarations
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
	t := &topo{}

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

type topo struct {
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

func (t *topo) N() int {
	return int(C.hwloc_get_nbobjs_by_type(t.ptr, C.HWLOC_OBJ_CORE))
}

func (t *topo) Distribute(
	n int, opts DistributeOptions) ([]string, error) {

	var (
		roots = C.hwloc_get_root_obj(t.ptr)
		depth = C.hwloc_get_type_or_below_depth(
			t.ptr, opts.Granularity.build())
		l = make([]C.hwloc_cpuset_t, n)
	)

	C.hwloc_distribute(
		t.ptr, roots, &l[0], C.uint(len(l)), C.uint(depth))

	r := make([]string, n)
	b := [64]C.char{}

	for i, c := range l {
		n := C.hwloc_bitmap_list_snprintf(&b[0], C.size_t(len(b)), c)
		r[i] = C.GoStringN(&b[0], n)
	}

	return r, nil
}
