// Copyright (c) 2016 Andrey Sibiryov <me@kobology.ru>
// Copyright (c) 2016 Other contributors as noted in the AUTHORS file.
//
// This file is part of Tesson.
//
// Tesson is free software; you can redistribute it and/or modify it under the
// terms of the GNU Lesser General Public License as published by the Free
// Software Foundation; either version 3 of the License, or (at your option)
// any later version.
//
// Tesson is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
// FOR A PARTICULAR PURPOSE. See the GNU Lesser General Public License for more
// details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

// Package tesson contains Tesson core implementation details.
//
// Tesson has several main abstractions: Topology, RuntimeContext and Frontend.
//
// Topology's responsible for gathering the information about hardware layout
// of the machine, analysing it and generating a deployment plan.
//
// RuntimeContext is an abstraction over execution engine, e.g. Docker Engine.
// It is supposed to spin up instances based on the deployment plan provided by
// Topology.
//
// Frontend is an optional component which represents a load balancer. It will
// be used to set up a virtual service, and aggregate all shards under a single
// endpoint.
//
// Default implementation is based on libhwloc, docker & gorb.
package tesson
