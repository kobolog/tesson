# Tesson [![Build Status][travis-img]][travis] [![GoDoc][godoc-img]][godoc]

**Shard All The Things!**

Tesson is a tool that automatically analyzes your hardware topology to utilize it as much as possible by spawning and pinning multiple instances of your app to available CPU cores and/or NUMA nodes, behind a local load balancer. Tesson is easily integrated with [Gorb](https://github.com/kobolog/gorb) to enable seamless, dynamic and extremely fast load balancing powered by [IPVS](https://en.wikipedia.org/wiki/IP_Virtual_Server).

Watch a talk about this tool here: [DockerCon 2016: Sharding Containers](https://www.youtube.com/watch?v=5lGVCPQeqiM).

## Configuration

Tesson has a decent built-in command line help system. If you don't like reading in the console, here's the gist of it. Tesson is a simple tool that can start, list and stop _sharded container groups_. A _sharded container group_ is a set of containers each being an identical instance of exactly the same container image. Such a group is usually fronted by a local load balancer so that external clients would still access it as an atomic entity.

The CLI is built after Git model, with a hierarchical command structure. To start a new _sharded container group_, use the `run` command:

    tesson run [-g <group-ident>] [-p <port-spec-1>, ..., -p <port-spec-N>] <image>

This command will automatically detect the underlying hardware architecture and spawn as many instances of the specified container image as it has physical cores. You can use additional flags and options to choose a different level of granularity (e.g. distribute among NUMA nodes, not CPU cores), override the number of instances and so on.

> Since Tesson relies on hardware topology to make decisions, it's important to understand that it has to be started on the same machine as the Docker daemon. Otherwise it will make decisions based on the wrong topology and ultimately fail to work.

In this example and further, `group-ident` can be anything that complies with the Docker container naming policy. This is the name that will be used to bundle containers together, to expose the sharded container group in local load balancer and as a service name for Consul registration, given the Gorb integration is enabled. If `group-ident` is not specified, a mangled image name will be used in place of it.

All the Docker-related options, apart from the image name and port bindings, can be provided via a config file in JSON format. The contents of this file must follow the format defined in [Docker API](https://docs.docker.com/engine/reference/api/docker_remote_api_v1.20/#create-a-container) documentation.

To see running sharded container groups, use the `ps` command:

    tesson ps

To stop a running sharded container group, use the `stop` command:

    tesson stop -g <group-ident>

## Gorb integration

To enable automatic frontend load balancer configuration and service discovery, you need to provide a Gorb URI via `--gorb` flag. The format is `device://endpoint:port`, e.g. `eth0://1.2.3.4:4672`. You must specify the device name for Tesson to know which address should be used to publish service ports on.

Alternatively, you can provide Gorb URI via an environment variable `GORB_URI`.

## TODO

- [x] Support for HostConfig for machine-specific user-defined configuration.
- [x] Better understanding of Docker container states: get rid of zombie instances in active groups.
- [ ] Use [macvlan Docker driver](https://github.com/docker/docker/blob/master/experimental/vlan-networks.md) & IPVS DR mode for local load balancing.
- [ ] Hardware device locality: allow pinning to NICs, disk subsystems, etc.
- [ ] More automation around resource quotas and management: CPU shares, memory limits (e.g. allow for memory reservation, etc).
- [ ] Support for remote usage: detect topology via hwloc container injection.

[travis]: https://travis-ci.org/kobolog/tesson
[travis-img]: https://travis-ci.org/kobolog/tesson.svg?branch=master
[godoc]: https://godoc.org/github.com/kobolog/tesson/lib
[godoc-img]: https://godoc.org/github.com/kobolog/tesson/lib?status.svg
