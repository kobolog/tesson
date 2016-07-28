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

package tesson

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/filters"
	"github.com/docker/go-connections/nat"

	log "github.com/Sirupsen/logrus"
)

// RuntimeContext represents an execution engine.
type RuntimeContext interface {
	Exec(group string, opts ExecOptions) (Group, error)
	List() ([]Group, error)
	Info(group string) (Group, error)
	Stop(group string, opts StopOptions) error
}

// Group represents runtime group status.
type Group struct {
	Name   string  // Human-readable group name.
	Image  string  // Container image name.
	Shards []Shard // Associated shards.
}

// Shard represents runtime shard status.
type Shard struct {
	Name   string // Human-readable shard name.
	ID     string // Unique shard ID.
	Status string // Status string.
	Unit   Unit   // Hardware layout.
	Ports  []types.Port
}

// ExecOptions specifies options for Exec.
type ExecOptions struct {
	Image  string   // Container image name.
	Layout []Unit   // Hardware layout.
	Ports  []string // Exposed ports to publish.
	Config string   // Container config file.
}

// StopOptions specifies options for Stop.
type StopOptions struct {
	Purge   bool          // Removes the container and its volumes.
	Timeout time.Duration // Timeout to SIGKILL.
}

// Implementation

// NewDockerContext constructs a new Docker-powered RuntimeContext.
func NewDockerContext(ctx context.Context) (RuntimeContext, error) {
	r, err := client.NewEnvClient()

	if err != nil {
		return nil, err
	}

	return &docker{ctx: ctx, client: r}, nil
}

type docker struct {
	ctx    context.Context
	client *client.Client
}

type config struct {
	container.Config
	HostConfig container.HostConfig
}

func (d *docker) Exec(group string, opts ExecOptions) (Group, error) {
	cfg := config{Config: container.Config{Labels: map[string]string{}}}

	if len(opts.Config) != 0 {
		b, err := ioutil.ReadFile(opts.Config)

		if err != nil {
			return Group{}, err
		}

		if err := json.Unmarshal(b, &cfg); err != nil {
			return Group{}, err
		}
	}

	var bindings nat.PortMap

	if cfg.HostConfig.PortBindings == nil && len(opts.Ports) > 0 {
		bindings = make(nat.PortMap)
	} else {
		bindings = cfg.HostConfig.PortBindings
	}

	for _, p := range opts.Ports {
		l, err := nat.ParsePortSpec(p)

		if err != nil {
			return Group{}, err
		}

		for _, n := range l {
			bindings[n.Port] = append(bindings[n.Port], n.Binding)
		}
	}

	cfg.Image = opts.Image
	cfg.HostConfig.PortBindings = bindings
	cfg.Labels["tesson.group"] = group

	for i, u := range opts.Layout {
		c := cfg // Copied for each unit to have a clean environment.

		c.HostConfig.Resources.CpusetCpus = u.String()
		c.Labels["tesson.unit.cpuset"] = u.String()
		c.Labels["tesson.unit.weight"] = strconv.Itoa(u.Weight())

		c.Env = append(c.Env, []string{
			fmt.Sprintf("GOMAXPROCS=%d", u.Weight()),
			fmt.Sprintf("TESSON_UID=%d", i)}...)

		if err := d.exec(group, types.ContainerCreateConfig{
			Config:     &c.Config,
			HostConfig: &c.HostConfig}, opts,
		); err != nil {
			return Group{}, err
		}
	}

	return d.Info(group)
}

func (d *docker) List() ([]Group, error) {
	f := filters.NewArgs()
	f.Add("label", "tesson.group")

	l, err := d.client.ContainerList(d.ctx, types.ContainerListOptions{
		All:    true,
		Filter: f,
	})

	if err != nil {
		return nil, err
	}

	m := make(map[string]*Group)

	for _, c := range l {
		var (
			label = c.Labels["tesson.group"]
			g     *Group
		)

		if g = m[label]; g == nil {
			g = &Group{Name: label, Image: c.Image}
			m[label] = g
		}

		g.Shards = append(g.Shards, d.convert(c))
	}

	var r []Group

	for _, v := range m {
		r = append(r, *v)
	}

	return r, nil
}

func (d *docker) Info(group string) (Group, error) {
	f := filters.NewArgs()
	f.Add("label", fmt.Sprintf("tesson.group=%s", group))

	l, err := d.client.ContainerList(d.ctx, types.ContainerListOptions{
		All:    true,
		Filter: f,
	})

	if err != nil {
		return Group{}, err
	}

	if len(l) == 0 {
		return Group{}, fmt.Errorf("group [%s] does not exist", group)
	}

	g := Group{Name: group, Image: l[0].Image}

	for _, c := range l {
		g.Shards = append(g.Shards, d.convert(c))
	}

	return g, nil
}

func (d *docker) Stop(group string, opts StopOptions) error {
	i, err := d.Info(group)

	if err != nil {
		return err
	}

	for _, shard := range i.Shards {
		if err := d.stop(group, shard.ID, opts); err != nil {
			return err
		}
	}

	return nil
}

func (d *docker) exec(
	group string, c types.ContainerCreateConfig, opts ExecOptions) error {

	r, err := d.client.ContainerCreate(d.ctx,
		c.Config, c.HostConfig, c.NetworkingConfig, c.Name)

	if err != nil {
		return err
	}

	log.Infof("instance created: %v.", r.ID)

	if err := d.client.ContainerStart(
		d.ctx, r.ID, types.ContainerStartOptions{},
	); err != nil {
		return err
	}

	return nil
}

func (d *docker) stop(group, id string, opts StopOptions) error {
	i, err := d.client.ContainerInspect(d.ctx, id)

	if err != nil {
		return err
	}

	if i.State.Running {
		if err :=
			d.client.ContainerStop(d.ctx, i.ID, opts.Timeout); err != nil {
			return err
		}

		log.Infof("instance stopped: %v.", i.ID)
	}

	if !opts.Purge {
		return nil
	}

	if err := d.client.ContainerRemove(
		d.ctx, i.ID, types.ContainerRemoveOptions{RemoveVolumes: true},
	); err != nil {
		return err
	}

	return nil
}

type unitInfo struct {
	CPUSet string
	NumCPU int
}

func (u unitInfo) String() string {
	return u.CPUSet
}

func (u unitInfo) Weight() int {
	return u.NumCPU
}

func (d *docker) convert(c types.Container) Shard {
	w, err := strconv.Atoi(c.Labels["tesson.unit.weight"])

	if err != nil {
		panic(err)
	}

	u := &unitInfo{CPUSet: c.Labels["tesson.unit.cpuset"], NumCPU: w}

	return Shard{
		Name:   strings.Join(c.Names, "; "),
		ID:     c.ID,
		Status: c.Status,
		Unit:   u,
		Ports:  c.Ports}
}
