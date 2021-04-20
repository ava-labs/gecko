package auth

import (
	"encoding/json"
	"net/http"

	rpc "github.com/gorilla/rpc/v2/json2"
)

type responseErr struct {
	Code    rpc.ErrorCode `json:"code"`
	Message string        `json:"message"`
}

type responseBody struct {
	Version string      `json:"jsonrpc"`
	Err     responseErr `json:"error"`
	ID      uint8       `json:"id"`
}

// Write a JSON-RPC formatted response saying that the API call is unauthorized.
// The response has header http.StatusUnauthorized.
// Errors while writing are ignored.
func writeUnauthorizedResponse(w http.ResponseWriter, err error) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	// There isn't anything to do with the returned error, so it is dropped.
	_ = json.NewEncoder(w).Encode(responseBody{
		Version: rpc.Version,
		Err: responseErr{
			Code:    rpc.E_INVALID_REQ,
			Message: err.Error(),
		},
		ID: 1,
	})
}
