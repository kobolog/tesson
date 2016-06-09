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

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/net/context"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/filters"
	"github.com/docker/go-connections/nat"

	log "github.com/Sirupsen/logrus"
)

var (
	errGroupDoesNotExist = errors.New("the specified group does not exist")
)

// RuntimeContext represents a execution runtime context.
type RuntimeContext interface {
	Exec(group string, opts ExecOptions) (Group, error)
	List() ([]Group, error)
	Info(group string) (Group, error)
	Stop(group string, opts StopOptions) error
}

// Group represents runtime group status.
type Group struct {
	Name   string  // Group name.
	Image  string  // Container image name.
	Shards []Shard // Group shards.
}

// Shard represents runtime group member status
type Shard struct {
	Name   string // Shard friendly name.
	ID     string // Shard ID.
	Status string // Shard status string.
	CPUs   string // Bound CPU cores.
	Ports  []types.Port
}

// ExecOptions specifies options for Exec.
type ExecOptions struct {
	Image  string   // Container image name.
	Layout []Unit   // CPU core indices to bind shards.
	Ports  []string // Published ports.
	Config string   // Container config file.
}

// StopOptions specifies options for Stop.
type StopOptions struct {
	Purge bool     // Whether to remove the container.
	Front Frontend // Local LB frontend.
}

// Implementation

// NewDockerContext returns a new Docker instance client.
func NewDockerContext(ctx context.Context) (RuntimeContext, error) {
	r, err := client.NewEnvClient()

	if err != nil {
		return nil, err
	}

	return &docker{ctx: ctx, client: r}, nil
}

func toShard(c types.Container) Shard {
	return Shard{
		Name:   c.Names[0],
		ID:     c.ID,
		CPUs:   c.Labels["tesson.shard"],
		Status: c.Status,
		Ports:  c.Ports}
}

type docker struct {
	ctx    context.Context
	client *client.Client
}

func (d *docker) Exec(group string, opts ExecOptions) (Group, error) {
	config := container.Config{Labels: map[string]string{}}

	if len(opts.Config) != 0 {
		f, err := os.Open(opts.Config)

		if err != nil {
			return Group{}, err
		}

		defer f.Close()

		if err := json.NewDecoder(bufio.NewReader(f)).Decode(
			&config,
		); err != nil {
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
		c.Labels["tesson.shard"] = u.String()

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

		g.Shards = append(g.Shards, toShard(c))
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
		return Group{}, errGroupDoesNotExist
	}

	g := Group{Name: group, Image: l[0].Image}

	for _, c := range l {
		g.Shards = append(g.Shards, toShard(c))
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
		return errGroupDoesNotExist
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

	log.Infof("instance created: %v", r.ID)

	if err := d.client.ContainerStart(
		d.ctx, r.ID, types.ContainerStartOptions{},
	); err != nil {
		return err
	}

	// i, err := d.client.ContainerInspect(d.ctx, r.ID)
	//
	// if err != nil {
	// 	return err
	// }

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

		log.Infof("instance stopped: %v", i.ID)
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
