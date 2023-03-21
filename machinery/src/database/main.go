package database

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"gopkg.in/mgo.v2"
)

type DB struct {
	Session *mgo.Session
}

var _init_ctx sync.Once
var _instance *DB
var DatabaseName = "KerberosFactory"

func New() *mgo.Session {
	host := os.Getenv("MONGODB_HOST")
	database := os.Getenv("MONGODB_DATABASE_CREDENTIALS")
	username := os.Getenv("MONGODB_USERNAME")
	password := os.Getenv("MONGODB_PASSWORD")

	_init_ctx.Do(func() {
		_instance = new(DB)
		mongoDBDialInfo := &mgo.DialInfo{
			Addrs:    strings.Split(host, ","),
			Timeout:  3 * time.Second,
			Database: database,
			Username: username,
			Password: password,
		}
		session, err := mgo.DialWithInfo(mongoDBDialInfo)
		if err != nil {
			log.Log.Error(fmt.Sprintf("Failed to connect to database: %s", err.Error()))
			os.Exit(1)
		}
		_instance.Session = session
	})

	return _instance.Session
}
