## Tesson [![Build Status](https://travis-ci.org/kobolog/tesson.svg?branch=master)](https://travis-ci.org/kobolog/tesson)

**Shard All The Things!**

Tesson is a tool that automatically analyzes your hardware topology to utilize it as much as possible by spawning and pinning multiple instances of your app to available CPU cores and/or NUMA nodes, behind a local load balancer. Tesson is easily integrated with [Gorb](https://github.com/kobolog/gorb) to enable seamless, dynamic and extremely fast load balancing powered by kernel IPVS technology.

## Configuration

Tesson has a decent built-in command line help system. If you don't like reading in the console, here's the gist of it. Tesson is a simple tool that can start, show and stop _sharded container groups_. A _sharded container group_ is a set of containers each being an identical instance of exactly the same container image. Such a group is usually fronted by a local load balancer so that external clients would still access it as an atomic entity.

The CLI is built after Git model, with a hierarchical command structure. To start a new _sharded container group_, use the `run` command:

    tesson run -g <group-name> -c <container-config-json>

This command will automatically detect the underlying hardware architecture and spawn as many instances as it has physical cores. You can use additional flags and options to choose a different level of granularity (e.g. distribute among NUMA nodes, not CPU cores) or override the number of instances.

In this example and further, `group name` can be anything that complies with the Docker container naming policy. This is the name that will be used to bundle containers together, to expose the sharded container group in local load balancer and as a service name for Consul registration, given the Gorb integration was enabled.

All the Docker-related options are provided via a config file in JSON format. The contents of this file must follow the format defined in [Docker API](https://docs.docker.com/engine/reference/api/docker_remote_api_v1.20/#create-a-container) documentation.

To see running sharded container groups, use the `list` command:

    tesson list

To stop a running sharded container group, use the `stop` command:

    tesson stop -g <group-name>

NOTE: since Tesson relies on hardware topology to make decisions, it's important to understand that it has to be started _on the same machine as the Docker daemon_. Otherwise it will make decisions based on the wrong topology and ultimately fail to work.

## TODO

- [ ] Support for HostConfig for machine-specific user-defined configuration.
- [ ] Better understanding of Docker container states: get rid of zombie instances in active groups.
- [ ] Hardware device locality: allow pinning to NICs, disk subsystems, etc.
- [ ] More automation around resource quotas and management: CPU shares, memory limits (e.g. allow for memory reservation, etc).
