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
	if !c.IsSet("name") || !c.IsSet("config") {
		return cli.ShowCommandHelp(c, "run")
	}

	var n int

	if c.Int("size") > 0 {
		n = c.Int("size")
	} else {
		n = t.N()
	}

	p, err := t.Distribute(n, tesson.DefaultDistribution())

	if err != nil {
		return err
	}

	log.Infof("sharding pattern: %s", strings.Join(p, ", "))

	return d.Exec(c.String("name"), c.String("config"), p)
}

func list(c *cli.Context) error {
	l, err := d.List()

	if err != nil {
		return err
	}

	if len(l) == 0 {
		log.Info("no sharded container groups found")
		return nil
	}

	for _, g := range l {
		n, _ := fmt.Printf("Group: %s (%s)\n", g.Name, g.Image)
		fmt.Println(strings.Repeat("-", n-1))

		for id, s := range g.Shards {
			fmt.Printf("|- [%s] %s (%s) bound to cores: %s\n",
				s.Status, s.Name, id[:6], s.CPUs)
		}

		fmt.Println()
	}

	return nil
}

func stop(c *cli.Context) error {
	if !c.IsSet("name") {
		return cli.ShowCommandHelp(c, "stop")
	}

	return d.Stop(c.String("name"), tesson.StopOptions{
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
			Name: "run",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "name",
					Aliases: []string{"g"},
					Usage:   "sharded container group name",
				},
				&cli.StringFlag{
					Name:    "config",
					Aliases: []string{"c"},
					Usage:   "container config",
				},
				&cli.IntFlag{
					Name:    "size",
					Aliases: []string{"n"},
					Usage:   "number of instances",
					Hidden:  true,
				},
			},
			Action: exec,
			Usage:  "start a sharded container group",
		},
		{
			Name:   "list",
			Action: list,
			Usage:  "list all active sharded container groups",
		},
		{
			Name: "stop",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "name",
					Aliases: []string{"g"},
					Usage:   "sharded container group name",
				},
				&cli.BoolFlag{
					Name:  "purge",
					Usage: "purge stopped containers",
				},
			},
			Action: stop,
			Usage:  "terminate a sharded container group",
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
