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
	Purge bool // Removes the container and its volumes.
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

func (d *docker) Exec(group string, opts ExecOptions) (Group, error) {
	config := container.Config{Labels: map[string]string{}}

	if len(opts.Config) != 0 {
		b, err := ioutil.ReadFile(opts.Config)

		if err != nil {
			return Group{}, err
		}

		if err := json.Unmarshal(b, &config); err != nil {
			return Group{}, err
		}
	}

	config.Image, config.Labels["tesson.group"] = opts.Image, group

	_, portBindings, err := nat.ParsePortSpecs(opts.Ports)

	if err != nil {
		return Group{}, err
	}

	for _, u := range opts.Layout {
		c := config // Cloned for every shard to have a clean environment.

		c.Env = append(c.Env, fmt.Sprintf("GOMAXPROCS=%d", u.Weight()))

		c.Labels["tesson.unit.string"] = u.String()
		c.Labels["tesson.unit.weight"] = strconv.Itoa(u.Weight())

		if err := d.exec(group, types.ContainerCreateConfig{
			Config: &c,
			HostConfig: &container.HostConfig{
				PortBindings: portBindings,
				Resources:    container.Resources{CpusetCpus: u.String()},
			}}, opts,
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

		g.Shards = append(g.Shards, _ContainerToShard(c))
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
		g.Shards = append(g.Shards, _ContainerToShard(c))
	}

	return g, nil
}

func (d *docker) Stop(group string, opts StopOptions) error {
	list, err := d.List()

	if err != nil {
		return err
	}

	var index int

	for index = 0; index < len(list); index++ {
		if list[index].Name == group {
			break
		}
	}

	if index >= len(list) {
		return fmt.Errorf("group [%s] does not exist", group)
	}

	for _, shard := range list[index].Shards {
		if err := d.stop(group, shard.ID, opts); err != nil {
			return err
		}
	}

	return nil
}

func (d *docker) exec(
	group string, cfg types.ContainerCreateConfig, opts ExecOptions) error {

	r, err := d.client.ContainerCreate(d.ctx,
		cfg.Config, cfg.HostConfig, cfg.NetworkingConfig, cfg.Name)

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
		if err := d.client.ContainerStop(d.ctx, i.ID, 30); err != nil {
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

type dockerUnit struct {
	unit   string
	weight int
}

func (u dockerUnit) String() string {
	return u.unit
}

func (u dockerUnit) Weight() int {
	return u.weight
}

func _ContainerToShard(c types.Container) Shard {
	w, err := strconv.Atoi(c.Labels["tesson.unit.weight"])

	if err != nil {
		panic(err)
	}

	u := &dockerUnit{unit: c.Labels["tesson.unit.string"], weight: w}

	return Shard{
		Name:   strings.Join(c.Names, "; "),
		ID:     c.ID,
		Status: c.Status,
		Unit:   u,
		Ports:  c.Ports}
}
