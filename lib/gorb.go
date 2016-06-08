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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/docker/go-connections/nat"

	"github.com/kobolog/gorb/pulse"
	"github.com/kobolog/gorb/util"
)

// Frontend represents a local load balancer.
type Frontend interface {
	CreateShard(service string, shard ShardOptions) error
	RemoveShard(service string, shard ShardOptions) error
}

// ShardOptions represents a single Tesson shard.
type ShardOptions struct {
	ID      string
	PortMap nat.PortMap
}

// Implementation

// NewGorbFrontend constructs a new Frontend powered by Gorb.
func NewGorbFrontend(uri string) (Frontend, error) {
	// URI is "device://host:port", e.g. "eth1://1.2.3.4:4872"
	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	g := &gorb{cache: make(map[string]struct{})}

	if addrs, err := util.InterfaceIPs(u.Scheme); err == nil {
		g.hostIPs = addrs
	} else {
		return nil, err
	}

	g.remote = &url.URL{Scheme: "http", Host: u.Host}

	return g, nil
}

type gorb struct {
	cache   map[string]struct{}
	hostIPs []net.IP
	remote  *url.URL
}

func (g *gorb) CreateShard(vs string, shard ShardOptions) error {
	for port, binds := range shard.PortMap {
		switch len(binds) {
		case 0:
			continue
		case 1:
			break
		default:
			return errors.New("multiple bindings not supported")
		}

		vsID := g.construct(vs, port)

		if _, ok := g.cache[vsID]; !ok {
			if err := g.createService(vsID, port); err != nil {
				return err
			}

			g.cache[vsID] = struct{}{}
		}

		if err := g.add(vsID, shard.ID, binds[0]); err != nil {
			return err
		}
	}

	return nil
}

type serviceRequest struct {
	Port     uint   `json:"port"`
	Protocol string `json:"protocol"`
}

func (g *gorb) createService(vsID string, p nat.Port) error {
	request := serviceRequest{
		Port: uint(p.Int()), Protocol: p.Proto()}

	u := *g.remote
	u.Path = path.Join("service", vsID)

	r, _ := http.NewRequest("PUT", u.String(), bytes.NewBuffer(
		util.MustMarshal(request, util.JSONOptions{})))

	return g.roundtrip(r, map[int]func() error{
		http.StatusConflict: func() error {
			return nil // not actually an error.
		}})
}

type backendRequest struct {
	Host  string         `json:"host"`
	Port  uint           `json:"port"`
	Pulse *pulse.Options `json:"pulse"`
}

func (g *gorb) add(vsID, rsID string, b nat.PortBinding) error {
	request := backendRequest{Host: b.HostIP}

	if request.Host == "0.0.0.0" {
		// Rewrite "catch-all" host to a real host's IP address.
		request.Host = g.hostIPs[0].String()
	}

	if n, err := strconv.Atoi(b.HostPort); err == nil {
		request.Port = uint(n)
	} else {
		return err
	}

	u := *g.remote
	u.Path = path.Join("service", vsID, rsID)

	r, _ := http.NewRequest("PUT", u.String(), bytes.NewBuffer(
		util.MustMarshal(request, util.JSONOptions{})))

	return g.roundtrip(r, map[int]func() error{
		http.StatusConflict: func() error {
			return fmt.Errorf("shard [%s] does exist", rsID)
		},
		http.StatusNotFound: func() error {
			return fmt.Errorf("service [%s] not found", vsID)
		}})
}

func (g *gorb) RemoveShard(vs string, shard ShardOptions) error {
	for p, binds := range shard.PortMap {
		switch len(binds) {
		case 0:
			continue
		case 1:
			break
		default:
			return errors.New("multiple bindings not supported")
		}

		u := *g.remote
		u.Path = path.Join("service", g.construct(vs, p), shard.ID)

		r, _ := http.NewRequest("DELETE", u.String(), nil)

		if err := g.roundtrip(r, map[int]func() error{
			http.StatusNotFound: func() error {
				return fmt.Errorf("shard [%s] not found", shard.ID)
			},
		}); err != nil {
			return err
		}
	}

	return nil
}

func (g *gorb) construct(vs string, p nat.Port) string {
	return fmt.Sprintf("%s-%s-%s", strings.Map(func(r rune) rune {
		switch r {
		case '/', ':':
			return '-'
		default:
			return r
		}
	}, vs), p.Port(), p.Proto())
}

type errorDispatch map[int]func() error

func (g *gorb) roundtrip(req *http.Request, ed errorDispatch) error {
	client := http.Client{}

	var r *http.Response

	r, err := client.Do(req)
	if err == nil {
		defer r.Body.Close()
	} else {
		return fmt.Errorf("gorb communication error: %s", err)
	}

	if r.StatusCode == http.StatusOK {
		return nil
	} else if h, exists := ed[r.StatusCode]; exists {
		return h()
	}

	// Some weird thing happened.
	var content interface{}

	if err := json.NewDecoder(r.Body).Decode(&content); err != nil {
		return fmt.Errorf("unknown error at %s", req.URL)
	}

	return fmt.Errorf("unknown error at %s: %v", req.URL, content)
}
