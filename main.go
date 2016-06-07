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

package main

import (
	"fmt"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/kobolog/tesson/lib"
	"gopkg.in/urfave/cli.v2"

	"golang.org/x/net/context"
)

var (
	d tesson.DockerContext
	t tesson.Topology
)

func exec(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.ShowCommandHelp(c, "run")
	}

	var (
		opts = tesson.ExecOptions{
			Image:  c.Args().Get(0),
			Ports:  c.StringSlice("port"),
			Config: c.String("config")}
		n    int
		name string
	)

	if c.Int("size") > 0 {
		n = c.Int("size")
	} else {
		n = t.N()
	}

	p, err := t.Distribute(n, tesson.DistributeOptions{
		Granularity: tesson.Core,
	})

	if err != nil {
		return err
	}

	if c.IsSet("group") {
		name = c.String("group")
	} else {
		name = opts.Image
	}

	log.Infof("spawning %d shards, layout: %s", len(p),
		strings.Join(p, ", "))

	return d.Exec(name, opts)
}

func list(c *cli.Context) error {
	l, err := d.List()

	if err != nil {
		return err
	}

	if len(l) == 0 {
		log.Info("no sharded container groups found!")
		return nil
	}

	for _, g := range l {
		n, _ := fmt.Printf("Group: %s (%s)\n", g.Name, g.Image)
		fmt.Println(strings.Repeat("-", n-1))

		for _, shard := range g.Shards {
			fmt.Printf(
				"|- [%s] %s (%s) unit layout: %s\n",
				shard.Status,
				shard.Name, shard.ID[:8], shard.CPUs)
		}

		fmt.Println()
	}

	return nil
}

func stop(c *cli.Context) error {
	if !c.IsSet("group") {
		return cli.ShowCommandHelp(c, "stop")
	}

	return d.Stop(c.String("group"), tesson.StopOptions{
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
					Hidden:  true,
				},
			},
			Action: exec,
		},
		{
			Usage:     "list all sharded container groups",
			ArgsUsage: " ",
			Name:      "ps",
			Action:    list,
		},
		{
			Usage:     "terminate a sharded container group",
			ArgsUsage: " ",
			Name:      "stop",
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

	d, err = tesson.NewDockerContext(context.Background())
	if err != nil {
		log.Fatalf("exec: %v", err)
	}

	t, err = tesson.NewHwlocTopology()
	if err != nil {
		log.Fatalf("topo: %v", err)
	}
}
