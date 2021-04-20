// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keystore

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/encdb"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/password"

	jsoncodec "github.com/ava-labs/avalanchego/utils/json"
)

const (
	// maxUserLen is the maximum allowed length of a username
	maxUserLen = 1024
)

var (
	errEmptyUsername = errors.New("empty username")
	errUserMaxLength = fmt.Errorf("username exceeds maximum length of %d chars", maxUserLen)

	_ Keystore = &keystore{}
)

type Keystore interface {
	// Create the API endpoint for this keystore.
	CreateHandler() (http.Handler, error)

	// NewBlockchainKeyStore returns this keystore limiting the functionality to
	// a single blockchain database.
	NewBlockchainKeyStore(blockchainID ids.ID) BlockchainKeystore

	// Get a database that is able to read and write unencrypted values from the
	// underlying database.
	GetDatabase(bID ids.ID, username, password string) (*encdb.Database, error)

	// Get the underlying database that is able to read and write encrypted
	// values. This Database will not perform any encrypting or decrypting of
	// values and is not recommended to be used when implementing a VM.
	GetRawDatabase(bID ids.ID, username, password string) (database.Database, error)

	// CreateUser attempts to register this username and password as a new user
	// of the keystore.
	CreateUser(username, pw string) error

	// DeleteUser attempts to remove the provided username and all of its data
	// from the keystore.
	DeleteUser(username, pw string) error

	// ListUsers returns all the users that currently exist in this keystore.
	ListUsers() ([]string, error)

	// ImportUser imports a serialized encoding of a user's information complete
	// with encrypted database values. The password is integrity checked.
	ImportUser(username, pw string, user []byte) error

	// ExportUser exports a serialized encoding of a user's information complete
	// with encrypted database values.
	ExportUser(username, pw string) ([]byte, error)

	// Get the password that is used by [username]. If [username] doesn't exist,
	// no error is returned and a nil password hash is returned.
	getPassword(username string) (*password.Hash, error)
}

type kvPair struct {
	Key   []byte `serialize:"true"`
	Value []byte `serialize:"true"`
}

// user describes the full content of a user
type user struct {
	password.Hash `serialize:"true"`
	Data          []kvPair `serialize:"true"`
}

// keystore implements keystore management logic
type keystore struct {
	lock sync.Mutex
	log  logging.Logger

	// Key: username
	// Value: The hash of that user's password
	usernameToPassword map[string]*password.Hash

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

func New(log logging.Logger, db database.Database) Keystore {
	return &keystore{
		log:                log,
		usernameToPassword: make(map[string]*password.Hash),
		userDB:             prefixdb.New([]byte("users"), db),
		bcDB:               prefixdb.New([]byte("bcs"), db),
	}
}

func (ks *keystore) CreateHandler() (http.Handler, error) {
	newServer := rpc.NewServer()
	codec := jsoncodec.NewCodec()
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	if err := newServer.RegisterService(&service{ks: ks}, "keystore"); err != nil {
		return nil, err
	}
	return newServer, nil
}

func (ks *keystore) NewBlockchainKeyStore(blockchainID ids.ID) BlockchainKeystore {
	return &blockchainKeystore{
		blockchainID: blockchainID,
		ks:           ks,
	}
}

func (ks *keystore) GetDatabase(bID ids.ID, username, password string) (*encdb.Database, error) {
	bcDB, err := ks.GetRawDatabase(bID, username, password)
	if err != nil {
		return nil, err
	}
	return encdb.New([]byte(password), bcDB)
}

func (ks *keystore) GetRawDatabase(bID ids.ID, username, pw string) (database.Database, error) {
	if username == "" {
		return nil, errEmptyUsername
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	passwordHash, err := ks.getPassword(username)
	if err != nil {
		return nil, err
	}
	if passwordHash == nil || !passwordHash.Check(pw) {
		return nil, fmt.Errorf("incorrect password for user %q", username)
	}

	userDB := prefixdb.New([]byte(username), ks.bcDB)
	bcDB := prefixdb.NewNested(bID[:], userDB)
	return bcDB, nil
}

func (ks *keystore) CreateUser(username, pw string) error {
	if username == "" {
		return errEmptyUsername
	}
	if len(username) > maxUserLen {
		return errUserMaxLength
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	passwordHash, err := ks.getPassword(username)
	if err != nil {
		return err
	}
	if passwordHash != nil {
		return fmt.Errorf("user already exists: %s", username)
	}

	if err := password.IsValid(pw, password.OK); err != nil {
		return err
	}

	passwordHash = &password.Hash{}
	if err := passwordHash.Set(pw); err != nil {
		return err
	}

	passwordBytes, err := c.Marshal(codecVersion, passwordHash)
	if err != nil {
		return err
	}

	if err := ks.userDB.Put([]byte(username), passwordBytes); err != nil {
		return err
	}
	ks.usernameToPassword[username] = passwordHash

	return nil
}

func (ks *keystore) DeleteUser(username, pw string) error {
	if username == "" {
		return errEmptyUsername
	}
	if len(username) > maxUserLen {
		return errUserMaxLength
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	// check if user exists and valid user.
	passwordHash, err := ks.getPassword(username)
	switch {
	case err != nil:
		return err
	case passwordHash == nil:
		return fmt.Errorf("user doesn't exist: %s", username)
	case !passwordHash.Check(pw):
		return fmt.Errorf("incorrect password for user %q", username)
	}

	userNameBytes := []byte(username)
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
	delete(ks.usernameToPassword, username)
	return nil
}

func (ks *keystore) ListUsers() ([]string, error) {
	users := []string{}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	it := ks.userDB.NewIterator()
	defer it.Release()
	for it.Next() {
		users = append(users, string(it.Key()))
	}
	return users, it.Error()
}

func (ks *keystore) ImportUser(username, pw string, userBytes []byte) error {
	if username == "" {
		return errEmptyUsername
	}
	if len(username) > maxUserLen {
		return errUserMaxLength
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	passwordHash, err := ks.getPassword(username)
	if err != nil {
		return err
	}
	if passwordHash != nil {
		return fmt.Errorf("user already exists: %s", username)
	}

	userData := user{}
	if _, err := c.Unmarshal(userBytes, &userData); err != nil {
		return err
	}
	if !userData.Hash.Check(pw) {
		return fmt.Errorf("incorrect password for user %q", username)
	}

	usrBytes, err := c.Marshal(codecVersion, &userData.Hash)
	if err != nil {
		return err
	}

	userBatch := ks.userDB.NewBatch()
	if err := userBatch.Put([]byte(username), usrBytes); err != nil {
		return err
	}

	userDataDB := prefixdb.New([]byte(username), ks.bcDB)
	dataBatch := userDataDB.NewBatch()
	for _, kvp := range userData.Data {
		if err := dataBatch.Put(kvp.Key, kvp.Value); err != nil {
			return fmt.Errorf("error on database put: %w", err)
		}
	}

	if err := atomic.WriteAll(dataBatch, userBatch); err != nil {
		return err
	}
	ks.usernameToPassword[username] = &userData.Hash
	return nil
}

func (ks *keystore) ExportUser(username, pw string) ([]byte, error) {
	if username == "" {
		return nil, errEmptyUsername
	}
	if len(username) > maxUserLen {
		return nil, errUserMaxLength
	}

	ks.lock.Lock()
	defer ks.lock.Unlock()

	passwordHash, err := ks.getPassword(username)
	if err != nil {
		return nil, err
	}
	if passwordHash == nil || !passwordHash.Check(pw) {
		return nil, fmt.Errorf("incorrect password for user %q", username)
	}

	userDB := prefixdb.New([]byte(username), ks.bcDB)

	userData := user{Hash: *passwordHash}
	it := userDB.NewIterator()
	defer it.Release()
	for it.Next() {
		userData.Data = append(userData.Data, kvPair{
			Key:   it.Key(),
			Value: it.Value(),
		})
	}
	if err := it.Error(); err != nil {
		return nil, err
	}

	// Return the byte representation of the user
	return c.Marshal(codecVersion, &userData)
}

func (ks *keystore) getPassword(username string) (*password.Hash, error) {
	// If the user is already in memory, return it
	passwordHash, exists := ks.usernameToPassword[username]
	if exists {
		return passwordHash, nil
	}

	// The user is not in memory; try the database
	userBytes, err := ks.userDB.Get([]byte(username))
	if err == database.ErrNotFound {
		// The user doesn't exist
		return nil, nil
	}
	if err != nil {
		// An unexpected database error occurred
		return nil, err
	}

	passwordHash = &password.Hash{}
	_, err = c.Unmarshal(userBytes, passwordHash)
	return passwordHash, err
}
