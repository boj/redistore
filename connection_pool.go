package redistore

import (
	"github.com/garyburd/redigo/redis"
)

// ConnectionPool
type ConnectionPool struct {
	size int
	conn chan interface{}
}

func (self *ConnectionPool) InitPool(size int, network, address string) error {
	self.size = size
	self.conn = make(chan interface{}, size)
	for i := 0; i < size; i++ {
		conn, err := self.CreateConnection(network, address)
		if err != nil {
			return err
		}
		self.conn <- conn
	}
	return nil
}

func (self *ConnectionPool) CreateConnection(network, address string) (interface{}, error) {
	c, err := redis.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (self *ConnectionPool) Close() {
	for i := 0; i < self.size; i++ {
		conn := self.GetConnection().(redis.Conn)
		conn.Close()
	}
}

func (self *ConnectionPool) GetConnection() interface{} {
	return <-self.conn
}

func (self *ConnectionPool) Releaseconnection(conn interface{}) {
	self.conn <- conn
}
