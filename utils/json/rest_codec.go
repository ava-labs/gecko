package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/gorilla/rpc/v2"
)

type URIMethodMap map[string]string
type RestCodec struct {
	Mapping URIMethodMap
}

type RestCodecRequest struct {
	*http.Request
	method string
}

var (
	RestBaseURL = "/rest"
)
// return list of rest endpoints
func (m *URIMethodMap) GetKeys() []string {
	var keys []string
	for k := range map[string]string(*m) {
		keys = append(keys, k)
	}
	return keys
}

//  Return rpc-method name for the rest api call
func (r *RestCodecRequest) Method() (string, error) {
	return r.method, nil
}

// generates mapping of uri to base.method
func MappingGenerator(m map[string]string, base string) URIMethodMap {
	r := URIMethodMap{}
	for k, v := range m {
		k = path.Join(RestBaseURL, k)
		r[k] = fmt.Sprintf("%s.%s", base, v)
	}
	return r
}
func (r *RestCodecRequest) ReadRequest(args interface{}) error {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Request.Body)
	if len(buf.Bytes()) == 0 {
		return nil
	}
	if err := json.Unmarshal(buf.Bytes(), args); err != nil {
		return err
	}

	return nil
}
func newEncode(w http.ResponseWriter) io.Writer {
	return w
}
func (c *RestCodecRequest) WriteResponse(w http.ResponseWriter, reply interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(newEncode(w))
	err := encoder.Encode(reply)

	if err != nil {
		rpc.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
func (codec RestCodec) NewRequest(req *http.Request) rpc.CodecRequest {

	for endpoint, methodName := range codec.Mapping {
		if strings.HasSuffix(strings.ToLower(req.RequestURI), endpoint) {
			return &RestCodecRequest{Request: req, method: methodName}
		}
	}
	return NewCodec().NewRequest(req)
}
func (c *RestCodecRequest) WriteError(w http.ResponseWriter, status int, err error) {
	rpc.WriteError(w, http.StatusInternalServerError, err.Error())
}
