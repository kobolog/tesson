package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kobolog/tesson/lib"

	log "github.com/sirupsen/logrus"
	"gopkg.in/urfave/cli.v2"
)

var (
	d tesson.Docker
	t tesson.Topology
)

func exec(c *cli.Context) error {
	var n int

	if !c.IsSet("name") || !c.IsSet("config") {
		return cli.ShowCommandHelp(c, "run")
	}

	if c.Int("size") > 0 {
		n = c.Int("size")
	} else {
		n = t.N()
	}

	topo, err := t.Distribute(n, tesson.DefaultDistribution())

	if err != nil {
		log.Fatalf("topo: %v", err)
	} else {
		log.Infof("sharding pattern: %s", strings.Join(topo, ", "))
	}

	if err := d.Exec(
		c.String("name"), c.String("config"), topo,
	); err != nil {
		log.Fatalf("exec: %v", err)
	}

	return nil
}

func stop(c *cli.Context) error {
	if !c.IsSet("name") {
		return cli.ShowCommandHelp(c, "stop")
	}

	if err := d.Kill(c.String("name")); err != nil {
		log.Fatalf("exec: %v", err)
	}

	return nil
}

func list(c *cli.Context) error {
	l, err := d.List()

	if err != nil {
		log.Fatalf("exec: %v", err)
	}

	for _, g := range l {
		n, _ := fmt.Printf("Group: %s (%s)\n", g.Name, g.Image)
		fmt.Println(strings.Repeat("-", n-1))

		for id, s := range g.Shards {
			fmt.Printf("|- [%s] %s (%s) bound to core(s) %s\n",
				s.Status, s.Name, id[:6], s.CPUs)
		}

		fmt.Println()
	}

	return nil
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
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run a sharded container group",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "name",
					Aliases: []string{"g"},
					Usage:   "sharded group name"},
				&cli.StringFlag{
					Name:    "config",
					Aliases: []string{"c"},
					Usage:   "container config"},
				&cli.IntFlag{
					Name:    "size",
					Aliases: []string{"n"},
					Usage:   "number of instances",
					Hidden:  true},
			},
			Action: exec,
		},
		{
			Name:    "stop",
			Aliases: []string{"s"},
			Usage:   "stop a sharded container group",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "name",
					Aliases: []string{"g"},
					Usage:   "sharded group name",
				},
			},
			Action: stop,
		},
		{
			Name:    "list",
			Aliases: []string{"l"},
			Usage:   "list all active sharded container groups",
			Action:  list,
		},
	}

	app.Run(os.Args)
}

func init() {
	var err error

	d, err = tesson.NewDocker()
	if err != nil {
		log.Fatalf("exec: %v", err)
	}

	t, err = tesson.NewHwlocTopology()
	if err != nil {
		log.Fatalf("topo: %v", err)
	}
}