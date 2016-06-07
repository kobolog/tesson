package tesson

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
)

var (
	errGroupDoesNotExist = errors.New("group does not exist")
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
	client *docker.Client
}

// NewDocker returns a new Docker instance client.
func NewDocker() (DockerContext, error) {
	r, err := docker.NewClientFromEnv()

	if err != nil {
		return nil, err
	}

	return &dockerCtx{client: r}, nil
}

func (d *dockerCtx) Exec(group, cfg string, topo []string) error {
	f, err := os.Open(cfg)

	if err != nil {
		return err
	}

	c := docker.Config{
		Labels: make(map[string]string),
	}

	if err := json.NewDecoder(bufio.NewReader(f)).Decode(
		&c,
	); err != nil {
		return err
	}

	f.Close()

	c.Labels["tesson.group"] = group

	for _, p := range topo {
		c.Labels["tesson.shard"] = p

		if err := d.exec(
			&c, &docker.HostConfig{CPUSetCPUs: p},
		); err != nil {
			return err
		}
	}

	return nil
}

func (d *dockerCtx) List() ([]Group, error) {
	list, err := d.client.ListContainers(docker.ListContainersOptions{
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
	l, err := d.List()

	if err != nil {
		return err
	}

	var idx int

	for idx = 0; idx < len(l); idx++ {
		if l[idx].Name == group {
			break
		}
	}

	if idx >= len(l) {
		return errGroupDoesNotExist
	}

	for id := range l[idx].Shards {
		if err := d.stop(id, opts); err != nil {
			return err
		}
	}

	return nil
}

func (d *dockerCtx) exec(cc *docker.Config, hc *docker.HostConfig) error {
	c, err := d.client.CreateContainer(docker.CreateContainerOptions{
		Config:     cc,
		HostConfig: hc,
	})

	if err != nil {
		return err
	}

	log.Infof("instance created: %v", c.ID)

	// TODO: use this response to configure Gorb w/o Link?
	return d.client.StartContainer(c.ID, nil)
}

func (d *dockerCtx) stop(id string, opts StopOptions) error {
	c, err := d.client.InspectContainer(id)

	if err != nil {
		return err
	}

	if c.State.Running {
		if err := d.client.StopContainer(c.ID, 30); err != nil {
			return err
		}

		log.Infof("instance stopped: %v", c.ID)
	}

	if !opts.Purge {
		return nil
	}

	return d.client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            c.ID,
		RemoveVolumes: true,
	})
}
