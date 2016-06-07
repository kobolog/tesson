package tesson

import (
	"bufio"
	"encoding/json"
	"os"

	gdc "github.com/fsouza/go-dockerclient"
	log "github.com/Sirupsen/logrus"
)

// Docker represends a Docker instance client.
type Docker interface {
	Exec(name, config string, topo []string) error
	List() ([]Group, error)
	Kill(name string) error
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

// Implementation

type docker struct {
	client *gdc.Client
}

// NewDocker returns a new Docker instance client.
func NewDocker() (Docker, error) {
	r, err := gdc.NewClientFromEnv()

	if err != nil {
		return nil, err
	}

	return &docker{client: r}, nil
}

func (d *docker) Exec(group, cfg string, topo []string) error {
	f, err := os.Open(cfg)

	if err != nil {
		return err
	}

	c := gdc.Config{
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

		if err := d.exec(p, &c); err != nil {
			return err
		}
	}

	return nil
}

func (d *docker) List() ([]Group, error) {
	l, err := d.client.ListContainers(gdc.ListContainersOptions{
		All: true,
	})

	if err != nil {
		return nil, err
	}

	m := make(map[string]*Group)

	for _, c := range l {
		var g *Group

		if groupID, ok := c.Labels["tesson.group"]; !ok {
			continue
		} else if g = m[groupID]; g == nil {
			g = &Group{
				Image: c.Image, Name: groupID,
				Shards: make(map[string]Shard)}

			m[groupID] = g
		}

		g.Shards[c.ID] = Shard{
			CPUs: c.Labels["tesson.shard"], Name: c.Names[0],
			Status: c.Status}
	}

	var r []Group

	for _, v := range m {
		r = append(r, *v)
	}

	return r, nil
}

func (d *docker) Kill(name string) error {
	l, err := d.List()

	if err != nil {
		return err
	}

	for _, g := range l {
		if g.Name == name {
			return d.kill(g)
		}
	}

	return nil
}

func (d *docker) exec(p string, cfg *gdc.Config) error {
	c, err := d.client.CreateContainer(gdc.CreateContainerOptions{
		Config:     cfg,
		HostConfig: &gdc.HostConfig{CPUSetCPUs: p},
	})

	if err != nil {
		return err
	}

	log.Infof("created %v", c.ID)

	// TODO: use this response to configure Gorb w/o Link?
	return d.client.StartContainer(c.ID, nil)
}

func (d *docker) kill(g Group) error {
	for id := range g.Shards {
		c, err := d.client.InspectContainer(id)

		if err != nil {
			return err
		}

		if !c.State.Running {
			continue
		}

		log.Infof("stopping %v", c.ID)

		if err := d.client.StopContainer(c.ID, 30); err != nil {
			return err
		}
	}

	return nil
}
