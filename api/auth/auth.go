package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/password"
	"github.com/ava-labs/avalanchego/utils/timer"

	cjson "github.com/ava-labs/avalanchego/utils/json"
)

const (
	headerKey      = "Authorization"
	headerValStart = "Bearer "

	// number of bytes to use when generating a new random token ID
	tokenIDByteLen = 20

	// defaultTokenLifespan is how long a token lives before it expires
	defaultTokenLifespan = time.Hour * 12

	maxEndpoints = 128
)

var (
	errNoToken               = errors.New("auth token not provided")
	errAuthHeaderNotParsable = fmt.Errorf(
		"couldn't parse auth token. Header \"%s\" should be \"%sTOKEN.GOES.HERE\"",
		headerKey,
		headerValStart,
	)
	errInvalidSigningMethod        = fmt.Errorf("auth token didn't specify the HS256 signing method correctly")
	errTokenRevoked                = errors.New("the provided auth token was revoked")
	errTokenInsufficientPermission = errors.New("the provided auth token does not allow access to this endpoint")

	errWrongPassword = errors.New("incorrect password")
	errSamePassword  = errors.New("new password can't be same as old password")
	errNoPassword    = errors.New("no password")

	errNoEndpoints      = errors.New("must name at least one endpoint")
	errTooManyEndpoints = fmt.Errorf("can only name at most %d endpoints", maxEndpoints)
)

type Auth interface {
	// Create and return a new token that allows access to each API endpoint for
	// [duration] such that the API's path ends with an element of [endpoints].
	// If one of the elements of [endpoints] is "*", all APIs are accessible.
	NewToken(pw string, duration time.Duration, endpoints []string) (string, error)

	// Revokes [token]; it will not be accepted as authorization for future API
	// calls. If the token is invalid, this is a no-op.  If a token is revoked
	// and then the password is changed, and then changed back to the current
	// password, the token will be un-revoked. Therefore, passwords shouldn't be
	// re-used before previously revoked tokens have expired.
	RevokeToken(pw, token string) error

	// Authenticates [token] for access to [url].
	AuthenticateToken(token, url string) error

	// Change the password required to create and revoke tokens.
	// [oldPW] is the current password.
	// [newPW] is the new password. It can't be the empty string and it can't be
	//         unreasonably long.
	// Changing the password makes tokens issued under a previous password
	// invalid.
	ChangePassword(oldPW, newPW string) error

	// Create the API endpoint for this auth handler.
	CreateHandler() (http.Handler, error)

	// WrapHandler wraps an http.Handler. Before passing a request to the
	// provided handler, the auth token is authenticated.
	WrapHandler(h http.Handler) http.Handler
}

type auth struct {
	// Used to mock time.
	clock timer.Clock

	log      logging.Logger
	endpoint string

	lock sync.RWMutex
	// Can be changed via API call.
	password password.Hash
	// Set of token IDs that have been revoked
	revoked map[string]struct{}
}

func New(log logging.Logger, endpoint, pw string) (Auth, error) {
	a := &auth{
		log:      log,
		endpoint: endpoint,
		revoked:  make(map[string]struct{}),
	}
	return a, a.password.Set(pw)
}

func NewFromHash(log logging.Logger, endpoint string, pw password.Hash) Auth {
	return &auth{
		log:      log,
		endpoint: endpoint,
		password: pw,
		revoked:  make(map[string]struct{}),
	}
}

func (a *auth) NewToken(pw string, duration time.Duration, endpoints []string) (string, error) {
	if pw == "" {
		return "", errNoPassword
	}
	if l := len(endpoints); l == 0 {
		return "", errNoEndpoints
	} else if l > maxEndpoints {
		return "", errTooManyEndpoints
	}

	a.lock.RLock()
	defer a.lock.RUnlock()

	if !a.password.Check(pw) {
		return "", errWrongPassword
	}

	canAccessAll := false
	for _, endpoint := range endpoints {
		if endpoint == "*" {
			canAccessAll = true
			break
		}
	}

	idBytes := [tokenIDByteLen]byte{}
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", fmt.Errorf("failed to generate the unique token ID due to %w", err)
	}
	id := base64.URLEncoding.EncodeToString(idBytes[:])

	claims := endpointClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: a.clock.Time().Add(duration).Unix(),
			Id:        id,
		},
	}
	if canAccessAll {
		claims.Endpoints = []string{"*"}
	} else {
		claims.Endpoints = endpoints
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	return token.SignedString(a.password.Password[:]) // Sign the token and return its string repr.
}

func (a *auth) RevokeToken(tokenStr, pw string) error {
	if tokenStr == "" {
		return errNoToken
	}
	if pw == "" {
		return errNoPassword
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	if !a.password.Check(pw) {
		return errWrongPassword
	}

	// See if token is well-formed and signature is right
	token, err := jwt.ParseWithClaims(tokenStr, &endpointClaims{}, a.getTokenKey)
	if err != nil {
		return err
	}

	// If the token isn't valid, it has essentially already been revoked.
	if !token.Valid {
		return nil
	}

	claims, ok := token.Claims.(*endpointClaims)
	if !ok {
		return fmt.Errorf("expected auth token's claims to be type endpointClaims but is %T", token.Claims)
	}
	a.revoked[claims.Id] = struct{}{}
	return nil
}

func (a *auth) AuthenticateToken(tokenStr, url string) error {
	a.lock.RLock()
	defer a.lock.RUnlock()

	token, err := jwt.ParseWithClaims(tokenStr, &endpointClaims{}, a.getTokenKey)
	if err != nil { // Probably because signature wrong
		return err
	}

	// Make sure this token gives access to the requested endpoint
	claims, ok := token.Claims.(*endpointClaims)
	if !ok {
		// Error is intentionally dropped here as there is nothing left to do
		// with it.
		return fmt.Errorf("expected auth token's claims to be type endpointClaims but is %T", token.Claims)
	}

	_, revoked := a.revoked[claims.Id]
	if revoked {
		return errTokenRevoked
	}

	for _, endpoint := range claims.Endpoints {
		if endpoint == "*" || strings.HasSuffix(url, endpoint) {
			return nil
		}
	}
	return errTokenInsufficientPermission
}

func (a *auth) ChangePassword(oldPW, newPW string) error {
	if oldPW == newPW {
		return errSamePassword
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	if !a.password.Check(oldPW) {
		return errWrongPassword
	}
	if err := password.IsValid(newPW, password.OK); err != nil {
		return err
	}
	if err := a.password.Set(newPW); err != nil {
		return err
	}

	// All the revoked tokens are now invalid; no need to mark specifically as
	// revoked.
	a.revoked = make(map[string]struct{})
	return nil
}

func (a *auth) CreateHandler() (http.Handler, error) {
	server := rpc.NewServer()
	codec := cjson.NewCodec()
	server.RegisterCodec(codec, "application/json")
	server.RegisterCodec(codec, "application/json;charset=UTF-8")
	return server, server.RegisterService(
		&service{auth: a},
		"auth",
	)
}

func (a *auth) WrapHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't require auth token to hit auth endpoint
		if path.Base(r.URL.Path) == a.endpoint {
			h.ServeHTTP(w, r)
			return
		}

		// Should be "Bearer AUTH.TOKEN.HERE"
		rawHeader := r.Header.Get(headerKey)
		if rawHeader == "" {
			writeUnauthorizedResponse(w, errNoToken)
			return
		}
		if !strings.HasPrefix(rawHeader, headerValStart) {
			// Error is intentionally dropped here as there is nothing left to
			// do with it.
			writeUnauthorizedResponse(w, errAuthHeaderNotParsable)
			return
		}
		// Returns actual auth token. Slice guaranteed to not go OOB
		tokenStr := rawHeader[len(headerValStart):]

		if err := a.AuthenticateToken(tokenStr, r.URL.Path); err != nil {
			writeUnauthorizedResponse(w, err)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// getTokenKey returns the key to use when making and parsing tokens
func (a *auth) getTokenKey(t *jwt.Token) (interface{}, error) {
	if t.Method != jwt.SigningMethodHS256 {
		return nil, errInvalidSigningMethod
	}
	return a.password.Password[:], nil
}
