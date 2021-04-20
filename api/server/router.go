// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package server

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
)

var (
	errUnknownBaseURL  = errors.New("unknown base url")
	errUnknownEndpoint = errors.New("unknown endpoint")
)

type router struct {
	lock   sync.RWMutex
	router *mux.Router

	routeLock      sync.Mutex
	reservedRoutes map[string]bool                    // Reserves routes so that there can't be alias that conflict
	aliases        map[string][]string                // Maps a route to a set of reserved routes
	routes         map[string]map[string]http.Handler // Maps routes to a handler
}

func newRouter() *router {
	return &router{
		router:         mux.NewRouter(),
		reservedRoutes: make(map[string]bool),
		aliases:        make(map[string][]string),
		routes:         make(map[string]map[string]http.Handler),
	}
}

func (r *router) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	r.router.ServeHTTP(writer, request)
}

func (r *router) GetHandler(base, endpoint string) (http.Handler, error) {
	r.routeLock.Lock()
	defer r.routeLock.Unlock()

	urlBase, exists := r.routes[base]
	if !exists {
		return nil, errUnknownBaseURL
	}
	handler, exists := urlBase[endpoint]
	if !exists {
		return nil, errUnknownEndpoint
	}
	return handler, nil
}

func (r *router) AddRouter(base, endpoint string, handler http.Handler) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.routeLock.Lock()
	defer r.routeLock.Unlock()

	return r.addRouter(base, endpoint, handler)
}

func (r *router) addRouter(base, endpoint string, handler http.Handler) error {
	if r.reservedRoutes[base] {
		return fmt.Errorf("couldn't route to %s as that route is either aliased or already maps to a handler", base)
	}

	return r.forceAddRouter(base, endpoint, handler)
}

func (r *router) forceAddRouter(base, endpoint string, handler http.Handler) error {
	endpoints := r.routes[base]
	if endpoints == nil {
		endpoints = make(map[string]http.Handler)
	}
	url := base + endpoint
	if _, exists := endpoints[endpoint]; exists {
		return fmt.Errorf("failed to create endpoint as %s already exists", url)
	}

	endpoints[endpoint] = handler
	r.routes[base] = endpoints
	// Name routes based on their URL for easy retrieval in the future
	if route := r.router.Handle(url, handler); route != nil {
		route.Name(url)
	} else {
		return fmt.Errorf("failed to create new route for %s", url)
	}

	var err error
	if aliases, exists := r.aliases[base]; exists {
		for _, alias := range aliases {
			if innerErr := r.forceAddRouter(alias, endpoint, handler); err == nil {
				err = innerErr
			}
		}
	}
	return err
}

func (r *router) AddAlias(base string, aliases ...string) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.routeLock.Lock()
	defer r.routeLock.Unlock()

	for _, alias := range aliases {
		if r.reservedRoutes[alias] {
			return fmt.Errorf("couldn't alias to %s as that route is either already aliased or already maps to a handler", alias)
		}
	}

	for _, alias := range aliases {
		r.reservedRoutes[alias] = true
	}

	r.aliases[base] = append(r.aliases[base], aliases...)

	var err error
	if endpoints, exists := r.routes[base]; exists {
		for endpoint, handler := range endpoints {
			for _, alias := range aliases {
				if innerErr := r.forceAddRouter(alias, endpoint, handler); err == nil {
					err = innerErr
				}
			}
		}
	}
	return err
}
