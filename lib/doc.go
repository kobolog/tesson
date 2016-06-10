// Package tesson contains Tesson core implementation details.
//
// It has three main abstractions:
// - Topology
// - RuntimeContext
// - Frontend
//
// Topology is responsible for gathering the information about hardware layout,
// analysing it and generating a deployment plan.
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
