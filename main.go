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

package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/net/context"

	"github.com/kobolog/tesson/lib"
	"gopkg.in/urfave/cli.v2"

	log "github.com/Sirupsen/logrus"
)

var (
	r tesson.RuntimeContext
	t tesson.Topology
)

func exec(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.ShowCommandHelp(c, "run")
	}

	var n int

	if c.Int("size") > 0 {
		n = c.Int("size")
	} else {
		n = t.N()
	}

	g, err := tesson.ParseGranularity(c.String("unit"))

	if err != nil {
		return err
	}

	l, err := t.Distribute(n, tesson.DistributeOptions{
		Granularity: g,
	})

	if err != nil {
		return err
	}

	opts := tesson.ExecOptions{
		Image:  c.Args().Get(0),
		Layout: l,
		Ports:  c.StringSlice("port"),
		Config: c.String("config")}

	var group string

	if c.IsSet("group") {
		group = c.String("group")
	} else {
		group = opts.Image
	}

	log.Infof("spawning %d shards, layout: %v", len(l), l)

	i, err := r.Exec(group, opts)

	if err != nil {
		return err
	}

	if c.IsSet("gorb") {
		f, err := tesson.NewGorbFrontend(c.String("gorb"))

		if err != nil {
			return err
		}

		return f.CreateService(group, i.Shards)
	}

	return nil
}

func list(c *cli.Context) error {
	l, err := r.List()

	if err != nil {
		return err
	}

	if len(l) == 0 {
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0)

	for i, g := range l {
		n, _ := fmt.Printf("Group: %s [%s]\n", g.Name, g.Image)
		fmt.Println(strings.Repeat("-", n-1))

		fmt.Fprintf(w, "INSTANCE ID\tSTATUS\tNAME\tLAYOUT\n")

		for _, s := range g.Shards {
			fmt.Fprintf(w, "%.8s\t%s\t%s\t%s\n", s.ID, s.Status,
				s.Name, s.CPUs)
		}

		w.Flush()

		if i < len(l)-1 {
			fmt.Println()
		}
	}

	return nil
}

func stop(c *cli.Context) error {
	if !c.IsSet("group") {
		return cli.ShowCommandHelp(c, "stop")
	}

	group := c.String("group")

	if c.IsSet("gorb") {
		f, err := tesson.NewGorbFrontend(c.String("gorb"))

		if err != nil {
			return err
		}

		i, err := r.Info(group)

		if err != nil {
			return err
		}

		if err := f.RemoveService(group, i.Shards); err != nil {
			return err
		}
	}

	return r.Stop(group, tesson.StopOptions{
		Purge: c.Bool("purge"),
	})
}

func main() {
	app := cli.NewApp()

	app.Authors = []*cli.Author{
		{Name: "Andrey Sibiryov", Email: "me@kobology.ru"}}

	app.Name = "Tesson"
	app.Usage = "Shard All The Things!"
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Usage:   "optional Gorb connection URI",
			Name:    "gorb",
			EnvVars: []string{"GORB_URI"},
		},
	}

	app.Commands = []*cli.Command{
		{
			Usage:     "start a sharded container group",
			ArgsUsage: "image",
			Name:      "run",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Usage:   "sharded container group name",
					Name:    "group",
					Aliases: []string{"g"},
				},
				&cli.StringFlag{
					Usage:   "container config",
					Name:    "config",
					Aliases: []string{"c"},
				},
				&cli.StringSliceFlag{
					Usage:   "ports to publish",
					Name:    "port",
					Aliases: []string{"p"},
				},
				&cli.IntFlag{
					Usage:   "number of instances",
					Name:    "size",
					Aliases: []string{"n"},
				},
				&cli.StringFlag{
					Usage:   "distribution unit",
					Name:    "unit",
					Aliases: []string{"u"},
					Value:   "core",
				},
			},
			Action: exec,
		},
		{
			Usage:  "list all sharded container groups",
			Name:   "ps",
			Action: list,
		},
		{
			Usage: "stop a sharded container group",
			Name:  "stop",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "group",
					Aliases: []string{"g"},
					Usage:   "sharded container group name",
				},
				&cli.BoolFlag{
					Name:  "purge",
					Usage: "purge stopped containers",
				},
			},
			Action: stop,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func init() {
	var err error

	r, err = tesson.NewDockerContext(context.Background())
	if err != nil {
		log.Fatalf("exec: %v", err)
	}

	t, err = tesson.NewHwlocTopology()
	if err != nil {
		log.Fatalf("topo: %v", err)
	}
}
