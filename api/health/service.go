// (c) 2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"net/http"
	"time"

	stdjson "encoding/json"

	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/gorilla/rpc/v2"
	"github.com/prometheus/client_golang/prometheus"

	health "github.com/AppsFlyer/go-sundheit"
	healthlib "github.com/ava-labs/avalanchego/health"
)

// Service wraps a [healthlib.Service]. Handler() returns a handler
// that handles incoming HTTP API requests. We have this in a separate
// package from [healthlib] to avoid a circular import where this service
// imports snow/engine/common but that package imports [healthlib].Checkable
type Service interface {
	healthlib.Service
	Handler() (*common.HTTPHandler, error)
}

func NewService(checkFreq time.Duration, log logging.Logger, namespace string, registry prometheus.Registerer) (Service, error) {
	service, err := healthlib.NewService(checkFreq, log, namespace, registry)
	if err != nil {
		return nil, err
	}
	return &apiServer{
		Service: service,
		log:     log,
	}, nil
}

// APIServer serves HTTP for a health service
type apiServer struct {
	healthlib.Service
	log logging.Logger
}

func (as *apiServer) Handler() (*common.HTTPHandler, error) {
	newServer := rpc.NewServer()
	codec := json.NewCodec()
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	if err := newServer.RegisterService(as, "health"); err != nil {
		return nil, err
	}

	// If a GET request is sent, we respond with a 200 if the node is healthy or
	// a 503 if the node isn't healthy.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			newServer.ServeHTTP(w, r)
			return
		}

		// Make sure the content type is set before writing the header.
		w.Header().Set("Content-Type", "application/json")

		checks, healthy := as.Results()
		if !healthy {
			// If a health check has failed, we should return a 503.
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		// The encoder will call write on the writer, which will write the
		// header with a 200.
		err := stdjson.NewEncoder(w).Encode(APIHealthReply{
			Checks:  checks,
			Healthy: healthy,
		})
		if err != nil {
			as.log.Debug("failed to encode the health check response due to %s", err)
		}
	})
	return &common.HTTPHandler{LockOptions: common.NoLock, Handler: handler}, nil
}

// APIHealthArgs are the arguments for Health
type APIHealthArgs struct{}

// APIHealthReply is the response for Health
type APIHealthReply struct {
	Checks  map[string]health.Result `json:"checks"`
	Healthy bool                     `json:"healthy"`
}

// Health returns a summation of the health of the node
func (as *apiServer) Health(_ *http.Request, _ *APIHealthArgs, reply *APIHealthReply) error {
	as.log.Info("Health.health called")
	reply.Checks, reply.Healthy = as.Results()
	if reply.Healthy {
		return nil
	}
	replyStr, err := stdjson.Marshal(reply.Checks)
	as.log.Warn("Health.health is returning an error: %s", string(replyStr))
	return err
}

// GetLiveness returns a summation of the health of the node
// Deprecated: in favor of Health
func (as *apiServer) GetLiveness(_ *http.Request, _ *APIHealthArgs, reply *APIHealthReply) error {
	as.log.Info("Health.getLiveness called")
	reply.Checks, reply.Healthy = as.Results()
	if reply.Healthy {
		return nil
	}
	replyStr, err := stdjson.Marshal(reply.Checks)
	as.log.Warn("Health.getLiveness is returning an error: %s", string(replyStr))
	return err
}

type noOp struct{}

// NewNoOpService returns a NoOp version of health check
// for when the Health API is disabled
func NewNoOpService() Service {
	return &noOp{}
}

// RegisterCheck implements the Service interface
func (n *noOp) Results() (map[string]health.Result, bool) {
	return map[string]health.Result{}, true
}

// RegisterCheck implements the Service interface
func (n *noOp) Handler() (_ *common.HTTPHandler, _ error) {
	return nil, nil
}

// RegisterCheckFn implements the Service interface
func (n *noOp) RegisterCheck(_ string, _ healthlib.Check) error {
	return nil
}

// RegisterMonotonicCheckFn implements the Service interface
func (n *noOp) RegisterMonotonicCheck(_ string, _ healthlib.Check) error {
	return nil
}
