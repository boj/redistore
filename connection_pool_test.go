package redistore

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"testing"
)

func TestConnectionPool(t *testing.T) {
	pool := new(ConnectionPool)
	if err := pool.InitPool(10, "tcp", ":6379"); err != nil {
		t.Error(err)
	}
	defer pool.Close()

	for i := 0; i < 100; i++ {
		go func(i int) {
			conn := pool.GetConnection().(redis.Conn)
			defer pool.Releaseconnection(conn)

			if _, err := conn.Do("SET", fmt.Sprintf("test:%d", i), fmt.Sprintf("test:%d", i)); err != nil {
				t.Error(err)
			}

			if _, err := redis.String(conn.Do("GET", fmt.Sprintf("test:%d", i))); err != nil {
				t.Error(err)
			}

			if _, err := conn.Do("DEL", fmt.Sprintf("test:%d", i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
}
