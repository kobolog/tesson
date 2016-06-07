package tesson

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"

	"golang.org/x/net/context"
)

var (
	errGroupDoesNotExist = errors.New("the specified group does not exist")
)

// DockerContext represends a Docker instance client.
type DockerContext interface {
	Exec(name, config string, topo []string) error
	List() ([]Group, error)
	Stop(name string, opts StopOptions) error
}

// Group represents runtime group information.
type Group struct {
	Image  string
	Name   string
	Shards map[string]Shard
}

// Shard represents a single group member.
type Shard struct {
	CPUs   string
	Name   string
	Status string
}

// StopOptions specifies options for Stop.
type StopOptions struct {
	Purge bool
}

// Implementation

type dockerCtx struct {
	ctx    context.Context
	client *client.Client
}

// NewDocker returns a new Docker instance client.
func NewDocker(ctx context.Context) (DockerContext, error) {
	r, err := client.NewEnvClient()

	if err != nil {
		return nil, err
	}

	return &dockerCtx{ctx: ctx, client: r}, nil
}

func (d *dockerCtx) Exec(group, config string, p []string) error {
	f, err := os.Open(config)

	if err != nil {
		return err
	}

	defer f.Close()

	c := container.Config{Labels: make(map[string]string)}

	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&c); err != nil {
		return err
	}

	c.Labels["tesson.group"] = group

	for _, shard := range p {
		c.Labels["tesson.shard"] = shard

		if err := d.exec(&c, &container.HostConfig{
			Resources: container.Resources{CpusetCpus: shard},
		}); err != nil {
			return err
		}
	}

	return nil
}

func (d *dockerCtx) List() ([]Group, error) {
	list, err := d.client.ContainerList(d.ctx, types.ContainerListOptions{
		All: true,
	})

	if err != nil {
		return nil, err
	}

	m := make(map[string]*Group)

	for _, c := range list {
		var g *Group

		if groupID, ok := c.Labels["tesson.group"]; !ok {
			continue
		} else if g = m[groupID]; g == nil {
			g = &Group{c.Image, groupID, make(map[string]Shard)}
			m[groupID] = g
		}

		g.Shards[c.ID] = Shard{
			c.Labels["tesson.shard"], c.Names[0], c.Status}
	}

	var r []Group

	for _, v := range m {
		r = append(r, *v)
	}

	return r, nil
}

func (d *dockerCtx) Stop(group string, opts StopOptions) error {
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

	for id := range list[index].Shards {
		if err := d.stop(id, opts); err != nil {
			return err
		}
	}

	return nil
}

func (d *dockerCtx) exec(c *container.Config, h *container.HostConfig) error {
	r, err := d.client.ContainerCreate(d.ctx, c, h, nil, "")

	if err != nil {
		return err
	}

	logrus.Infof("instance created: %v", r.ID)

	// TODO: use this response to configure Gorb w/o Link?
	return d.client.ContainerStart(d.ctx, r.ID, types.ContainerStartOptions{})
}

func (d *dockerCtx) stop(id string, opts StopOptions) error {
	r, err := d.client.ContainerInspect(d.ctx, id)

	if err != nil {
		return err
	}

	if r.State.Running {
		if err := d.client.ContainerStop(d.ctx, r.ID, 30); err != nil {
			return err
		}

		logrus.Infof("instance stopped: %v", r.ID)
	}

	if !opts.Purge {
		return nil
	}

	return d.client.ContainerRemove(d.ctx, r.ID, types.ContainerRemoveOptions{
		RemoveVolumes: true,
	})
}
