package auth

import (
	"net/http"

	"github.com/ava-labs/avalanchego/api"
)

// service that serves the Auth API functionality.
type service struct {
	auth *auth
}

type Password struct {
	Password string `json:"password"` // The authorization password
}

type NewTokenArgs struct {
	Password
	// Endpoints that may be accessed with this token e.g. if endpoints is
	// ["/ext/bc/X", "/ext/admin"] then the token holder can hit the X-Chain API
	// and the admin API. If [Endpoints] contains an element "*" then the token
	// allows access to all API endpoints. [Endpoints] must have between 1 and
	// [maxEndpoints] elements
	Endpoints []string `json:"endpoints"`
}

type Token struct {
	Token string `json:"token"` // The new token. Expires in [TokenLifespan].
}

func (s *service) NewToken(_ *http.Request, args *NewTokenArgs, reply *Token) error {
	s.auth.log.Info("Auth: NewToken called")

	var err error
	reply.Token, err = s.auth.NewToken(args.Password.Password, defaultTokenLifespan, args.Endpoints)
	return err
}

type RevokeTokenArgs struct {
	Password
	Token
}

func (s *service) RevokeToken(_ *http.Request, args *RevokeTokenArgs, reply *api.SuccessResponse) error {
	s.auth.log.Info("Auth: RevokeToken called")

	reply.Success = true
	return s.auth.RevokeToken(args.Token.Token, args.Password.Password)
}

type ChangePasswordArgs struct {
	OldPassword string `json:"oldPassword"` // Current authorization password
	NewPassword string `json:"newPassword"` // New authorization password
}

func (s *service) ChangePassword(_ *http.Request, args *ChangePasswordArgs, reply *api.SuccessResponse) error {
	s.auth.log.Info("Auth: ChangePassword called")

	reply.Success = true
	return s.auth.ChangePassword(args.OldPassword, args.NewPassword)
}
