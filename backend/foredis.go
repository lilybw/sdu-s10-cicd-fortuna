package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
)

var dbLink redis.Conn
var usingRedis = false

func init() {
	// Check if REDIS_DNS environment variable is set
	if os.Getenv("REDIS_DNS") == "" {
		fmt.Println("redis config not set")
		return
	}
	var err error
	for i := 0; i < 5; i++ {
		dbLink, err = redis.Dial("tcp", fmt.Sprintf("%s:6379", getEnv("REDIS_DNS", "localhost")))
		if err == nil {
			usingRedis = true
			break
		}
		log.Printf("Attempt %d: redis connection failed: %s", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if !usingRedis {
		log.Println("Failed to connect to redis after 5 attempts")
		return
	}

	resKeys, err := redis.Values(dbLink.Do("hkeys", "fortunes"))
	if err != nil {
		fmt.Println("redis hkeys failed", err.Error())
		return
	}

	// Only replace the in-memory fortunes with what's in Redis if Redis
	// actually HAS fortunes stored. A fresh Redis instance with nothing
	// in it should not wipe out the built-in default fortunes (or
	// anything already loaded from SQLite) - it should just leave them
	// alone until something gets added.
	if len(resKeys) == 0 {
		fmt.Println("redis has no fortunes yet, keeping existing fortunes")
		return
	}

	datastoreDefault = datastore{m: map[string]fortune{}, RWMutex: &sync.RWMutex{}}
	fmt.Printf("*** loading redis fortunes:\n")
	for _, key := range resKeys {
		val, err := dbLink.Do("hget", "fortunes", key)
		if err != nil {
			fmt.Println("redis hget failed", err.Error())
		} else {
			idx := fmt.Sprintf("%s", key.([]byte))
			msg := fmt.Sprintf("%s", val.([]byte))
			datastoreDefault.m[idx] = fortune{ID: idx, Message: msg}
			fmt.Printf("%s => %s\n", key, val)
		}
	}
}
