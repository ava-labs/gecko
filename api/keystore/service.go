// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keystore

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ava-labs/avalanchego/api/apiargs"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/encdb"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/codec"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/password"

	jsoncodec "github.com/ava-labs/avalanchego/utils/json"
)

const (
	// maxUserLen is the maximum allowed length of a username
	maxUserLen = 1024

	maxPackerSize  = 1 << 30 // max size, in bytes, of something being marshalled by Marshal()
	maxSliceLength = 1 << 18

	codecVersion = 0
)

var (
	errEmptyUsername = errors.New("empty username")
	errUserMaxLength = fmt.Errorf("username exceeds maximum length of %d chars", maxUserLen)
)

// KeyValuePair ...
type KeyValuePair struct {
	Key   []byte `serialize:"true"`
	Value []byte `serialize:"true"`
}

// UserDB describes the full content of a user
type UserDB struct {
	password.Hash `serialize:"true"`
	Data          []KeyValuePair `serialize:"true"`
}

// Keystore is the RPC interface for keystore management
type Keystore struct {
	lock  sync.Mutex
	log   logging.Logger
	codec codec.Manager

	// Key: username
	// Value: The user with that name
	users map[string]*password.Hash

	// Used to persist users and their data
	userDB database.Database
	bcDB   database.Database
	//           BaseDB
	//          /      \
	//    UserDB        BlockchainDB
	//                 /      |     \
	//               Usr     Usr    Usr
	//             /  |  \
	//          BID  BID  BID
}

// Initialize the keystore
func (ks *Keystore) Initialize(log logging.Logger, db database.Database) error {
	c := codec.New(codec.DefaultTagName, maxSliceLength)
	manager := codec.NewManager(maxPackerSize)
	if err := manager.RegisterCodec(codecVersion, c); err != nil {
		return err
	}

	ks.log = log
	ks.codec = manager
	ks.users = make(map[string]*password.Hash)
	ks.userDB = prefixdb.New([]byte("users"), db)
	ks.bcDB = prefixdb.New([]byte("bcs"), db)
	return nil
}

// CreateHandler returns a new service object that can send requests to thisAPI.
func (ks *Keystore) CreateHandler() (*common.HTTPHandler, error) {
	newServer := rpc.NewServer()
	codec := jsoncodec.NewCodec()
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	if err := newServer.RegisterService(ks, "keystore"); err != nil {
		return nil, err
	}
	return &common.HTTPHandler{LockOptions: common.NoLock, Handler: newServer}, nil
}

// Get the user whose name is [username]
func (ks *Keystore) getUser(username string) (*password.Hash, error) {
	// If the user is already in memory, return it
	user, exists := ks.users[username]
	if exists {
		return user, nil
	}
	// The user is not in memory; try the database
	userBytes, err := ks.userDB.Get([]byte(username))
	if err != nil { // Most likely bc user doesn't exist in database
		return nil, err
	}

	user = &password.Hash{}
	_, err = ks.codec.Unmarshal(userBytes, user)
	return user, err
}

// CreateUser creates an empty user with the provided username and password
func (ks *Keystore) CreateUser(_ *http.Request, args *apiargs.UserPass, reply *apiargs.SuccessResponse) error {
	ks.log.Info("Keystore: CreateUser called with %.*s", maxUserLen, args.Username)

	ks.lock.Lock()
	defer ks.lock.Unlock()

	if err := ks.AddUser(args.Username, args.Password); err != nil {
		return err
	}

	reply.Success = true
	return nil
}

// ListUsersReply is the reply from ListUsers
type ListUsersReply struct {
	Users []string `json:"users"`
}

// ListUsers lists all the registered usernames
func (ks *Keystore) ListUsers(_ *http.Request, args *struct{}, reply *ListUsersReply) error {
	ks.log.Info("Keystore: ListUsers called")

	reply.Users = []string{}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	it := ks.userDB.NewIterator()
	defer it.Release()
	for it.Next() {
		reply.Users = append(reply.Users, string(it.Key()))
	}
	return it.Error()
}

// ExportUserArgs ...
type ExportUserArgs struct {
	// The username and password
	apiargs.UserPass
	// The encoding for the exported user ("hex" or "cb58")
	Encoding formatting.Encoding `json:"encoding"`
}

// ExportUserReply is the reply from ExportUser
type ExportUserReply struct {
	// String representation of the user
	User string `json:"user"`
	// The encoding for the exported user ("hex" or "cb58")
	Encoding formatting.Encoding `json:"encoding"`
}

// ExportUser exports a serialized encoding of a user's information complete with encrypted database values
func (ks *Keystore) ExportUser(_ *http.Request, args *ExportUserArgs, reply *ExportUserReply) error {
	ks.log.Info("Keystore: ExportUser called for %s", args.Username)

	ks.lock.Lock()
	defer ks.lock.Unlock()

	user, err := ks.getUser(args.Username)
	if err != nil {
		return err
	}
	if !user.Check(args.Password) {
		return fmt.Errorf("incorrect password for user %q", args.Username)
	}

	userDB := prefixdb.New([]byte(args.Username), ks.bcDB)

	userData := UserDB{Hash: *user}

	it := userDB.NewIterator()
	defer it.Release()
	for it.Next() {
		userData.Data = append(userData.Data, KeyValuePair{
			Key:   it.Key(),
			Value: it.Value(),
		})
	}
	if err := it.Error(); err != nil {
		return err
	}

	// Get byte representation of user
	b, err := ks.codec.Marshal(codecVersion, &userData)
	if err != nil {
		return err
	}

	// Encode the user from bytes to string
	reply.User, err = formatting.Encode(args.Encoding, b)
	if err != nil {
		return fmt.Errorf("couldn't encode user to string: %w", err)
	}
	reply.Encoding = args.Encoding
	return nil
}

// ImportUserArgs are arguments for ImportUser
type ImportUserArgs struct {
	// The username and password of the user being imported
	apiargs.UserPass
	// The string representation of the user
	User string `json:"user"`
	// The encoding of [User] ("hex" or "cb58")
	Encoding formatting.Encoding `json:"encoding"`
}

// ImportUser imports a serialized encoding of a user's information complete with encrypted database values,
// integrity checks the password, and adds it to the database
func (ks *Keystore) ImportUser(r *http.Request, args *ImportUserArgs, reply *apiargs.SuccessResponse) error {
	ks.log.Info("Keystore: ImportUser called for %s", args.Username)

	if args.Username == "" {
		return errEmptyUsername
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	// Decode the user from string to bytes
	userBytes, err := formatting.Decode(args.Encoding, args.User)
	if err != nil {
		return fmt.Errorf("couldn't decode 'user' to bytes: %w", err)
	}

	if usr, err := ks.getUser(args.Username); err == nil || usr != nil {
		return fmt.Errorf("user already exists: %s", args.Username)
	}

	userData := UserDB{}
	if _, err := ks.codec.Unmarshal(userBytes, &userData); err != nil {
		return err
	}
	if !userData.Hash.Check(args.Password) {
		return fmt.Errorf("incorrect password for user %q", args.Username)
	}

	usrBytes, err := ks.codec.Marshal(codecVersion, &userData.Hash)
	if err != nil {
		return err
	}

	userBatch := ks.userDB.NewBatch()
	if err := userBatch.Put([]byte(args.Username), usrBytes); err != nil {
		return err
	}

	userDataDB := prefixdb.New([]byte(args.Username), ks.bcDB)
	dataBatch := userDataDB.NewBatch()
	for _, kvp := range userData.Data {
		if err := dataBatch.Put(kvp.Key, kvp.Value); err != nil {
			return fmt.Errorf("error on database put: %w", err)
		}
	}

	if err := atomic.WriteAll(dataBatch, userBatch); err != nil {
		return err
	}

	ks.users[args.Username] = &userData.Hash

	reply.Success = true
	return nil
}

// DeleteUser deletes user with the provided username and password.
func (ks *Keystore) DeleteUser(_ *http.Request, args *apiargs.UserPass, reply *apiargs.SuccessResponse) error {
	ks.log.Info("Keystore: DeleteUser called with %s", args.Username)

	if args.Username == "" {
		return errEmptyUsername
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	// check if user exists and valid user.
	usr, err := ks.getUser(args.Username)
	switch {
	case err != nil || usr == nil:
		return fmt.Errorf("user doesn't exist: %s", args.Username)
	case !usr.Check(args.Password):
		return fmt.Errorf("incorrect password for user %q", args.Username)
	}

	userNameBytes := []byte(args.Username)
	userBatch := ks.userDB.NewBatch()
	if err := userBatch.Delete(userNameBytes); err != nil {
		return err
	}

	userDataDB := prefixdb.New(userNameBytes, ks.bcDB)
	dataBatch := userDataDB.NewBatch()

	it := userDataDB.NewIterator()
	defer it.Release()

	for it.Next() {
		if err = dataBatch.Delete(it.Key()); err != nil {
			return err
		}
	}

	if err = it.Error(); err != nil {
		return err
	}

	if err := atomic.WriteAll(dataBatch, userBatch); err != nil {
		return err
	}

	// delete from users map.
	delete(ks.users, args.Username)

	reply.Success = true
	return nil
}

// NewBlockchainKeyStore ...
func (ks *Keystore) NewBlockchainKeyStore(blockchainID ids.ID) *BlockchainKeystore {
	return &BlockchainKeystore{
		blockchainID: blockchainID,
		ks:           ks,
	}
}

// GetDatabase ...
func (ks *Keystore) GetDatabase(bID ids.ID, username, password string) (database.Database, error) {
	ks.log.Info("Keystore: GetDatabase called with %s from %s", username, bID)

	ks.lock.Lock()
	defer ks.lock.Unlock()

	usr, err := ks.getUser(username)
	if err != nil {
		return nil, err
	}
	if !usr.Check(password) {
		return nil, fmt.Errorf("incorrect password for user %q", username)
	}

	userDB := prefixdb.New([]byte(username), ks.bcDB)
	bcDB := prefixdb.NewNested(bID[:], userDB)
	return encdb.New([]byte(password), bcDB)
}

// AddUser attempts to register this username and password as a new user of the
// keystore.
func (ks *Keystore) AddUser(username, pword string) error {
	if username == "" {
		return errEmptyUsername
	}
	if len(username) > maxUserLen {
		return errUserMaxLength
	}

	if user, err := ks.getUser(username); err == nil || user != nil {
		return fmt.Errorf("user already exists: %s", username)
	}

	if err := password.IsValid(pword, password.OK); err != nil {
		return err
	}

	user := &password.Hash{}
	if err := user.Set(pword); err != nil {
		return err
	}

	userBytes, err := ks.codec.Marshal(codecVersion, user)
	if err != nil {
		return err
	}

	if err := ks.userDB.Put([]byte(username), userBytes); err != nil {
		return err
	}
	ks.users[username] = user

	return nil
}

// CreateTestKeystore returns a new keystore that can be utilized for testing
func CreateTestKeystore() (*Keystore, error) {
	ks := &Keystore{}
	return ks, ks.Initialize(logging.NoLog{}, memdb.New())
}
