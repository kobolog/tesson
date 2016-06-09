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
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/docker/engine-api/types"

	"github.com/kobolog/gorb/pulse"
	"github.com/kobolog/gorb/util"

	log "github.com/Sirupsen/logrus"
)

// Frontend represents a local load balancer.
type Frontend interface {
	CreateService(group string, shards []Shard) error
	RemoveService(group string, shards []Shard) error
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

	g.url = &url.URL{Scheme: "http", Host: u.Host}

	return g, nil
}

type gorb struct {
	cache   map[string]struct{}
	hostIPs []net.IP
	url     *url.URL
}

func (g *gorb) CreateService(group string, shards []Shard) error {
	for _, shard := range shards {
		for _, port := range shard.Ports {
			if port.PublicPort == 0 || port.PrivatePort == 0 {
				continue
			}

			vsID := g.mangle(group, port)

			if _, ok := g.cache[vsID]; !ok {
				if err := g.createService(vsID, port); err != nil {
					return err
				}

				g.cache[vsID] = struct{}{}
			}

			rsID := g.mangle(shard.ID, port)

			if err := g.createBackend(vsID, rsID, port); err != nil {
				return err
			}
		}
	}

	return nil
}

type serviceRequest struct {
	Port     uint   `json:"port"`
	Protocol string `json:"protocol"`
}

func (g *gorb) createService(vsID string, p types.Port) error {
	log.Infof("registering group service: %s", vsID)

	request := serviceRequest{
		Port: uint(p.PrivatePort), Protocol: p.Type}

	u := *g.url
	u.Path = path.Join("service", vsID)

	r, _ := http.NewRequest("PUT", u.String(), bytes.NewBuffer(
		util.MustMarshal(request, util.JSONOptions{})))

	return g.roundtrip(r, errorDispatch{
		http.StatusConflict: func() error {
			return nil // not actually an error.
		}})
}

type backendRequest struct {
	Host  string         `json:"host"`
	Port  uint           `json:"port"`
	Pulse *pulse.Options `json:"pulse"`
}

func (g *gorb) createBackend(vsID, rsID string, p types.Port) error {
	log.Infof("registering shard: %s/%s", vsID, rsID)

	request := backendRequest{
		Host: p.IP, Port: uint(p.PublicPort)}

	if p.Type == "udp" {
		// Disable health checks for UDP-based services.
		request.Pulse = &pulse.Options{Type: "none"}
	}

	if request.Host == "0.0.0.0" {
		// TODO: generate for each interface address.
		request.Host = g.hostIPs[0].String()
	}

	u := *g.url
	u.Path = path.Join("service", vsID, rsID)

	r, _ := http.NewRequest("PUT", u.String(), bytes.NewBuffer(
		util.MustMarshal(request, util.JSONOptions{})))

	return g.roundtrip(r, errorDispatch{
		http.StatusConflict: func() error {
			return fmt.Errorf("shard [%s] does exist", rsID)
		},
		http.StatusNotFound: func() error {
			return fmt.Errorf("service [%s] not found", vsID)
		}})
}

func (g *gorb) RemoveService(group string, shards []Shard) error {
	vsIDs := map[string]struct{}{}

	for _, shard := range shards {
		for _, port := range shard.Ports {
			if port.PublicPort == 0 || port.PrivatePort == 0 {
				continue
			}

			vsIDs[g.mangle(group, port)] = struct{}{}
		}
	}

	u := *g.url

	for vsID := range vsIDs {
		log.Infof("withdrawing group service registration: %s", vsID)

		u.Path = path.Join("service", vsID)
		r, _ := http.NewRequest("DELETE", u.String(), nil)

		if err := g.roundtrip(r, errorDispatch{
			http.StatusNotFound: func() error {
				return fmt.Errorf("service [%s] not found", vsID)
			}},
		); err != nil {
			return err
		}

		delete(g.cache, vsID)
	}

	return nil
}

func (g *gorb) mangle(id string, p types.Port) string {
	return fmt.Sprintf("%s-%d-%s", strings.Map(func(r rune) rune {
		switch r {
		case '/', ':':
			return '-'
		default:
			return r
		}
	}, id), p.PrivatePort, p.Type)
}

type errorDispatch map[int]func() error

func (g *gorb) roundtrip(req *http.Request, ed errorDispatch) error {
	client := http.Client{}
	r, err := client.Do(req)

	if err == nil {
		defer r.Body.Close()
	} else {
		return fmt.Errorf("http error: %s", err)
	}

	if r.StatusCode == http.StatusOK {
		return nil
	} else if h, exists := ed[r.StatusCode]; exists {
		return h()
	}

	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		return err
	}

	return fmt.Errorf("unknown gorb error at %s: %s", req.URL, body)
}
