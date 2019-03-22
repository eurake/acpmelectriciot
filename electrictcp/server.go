/******************************************************
# DESC    : tcp服务器,
# AUTHOR  : Lwdsun
# EMAIL   : lwdsun@sina.com
# MOD     : 2018-06-05
			2018-07-23 此处需要优化tcp map的并发问题, 暂时
			           不做处理, 留着以后处理
# FILE    : server.go
******************************************************/
package electrictcp

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/eurake/acpm/config"
)

type CollectConns struct {
	newmutex sync.Mutex
	clients  map[string]*Client
}

// 添加一个客户端
func (c *CollectConns) Add(client *Client) {
	c.newmutex.Lock()
	c.clients[client.uuid] = client
	c.newmutex.Unlock()
}

// 删除一个客户端
func (c *CollectConns) Remove(uuid string) {
	c.newmutex.Lock()
	if _, ok := c.clients[uuid]; ok {
		delete(c.clients, uuid)
	}
	c.newmutex.Unlock()
}

// 得到一个key的客户端
func (c *CollectConns) Get(uuid string) (*Client, error) {
	c.newmutex.Lock()
	defer c.newmutex.Unlock()
	if _, ok := c.clients[uuid]; ok {
		return c.clients[uuid], nil
	}
	return nil, errors.New("not exist")
}

// 返回当前有多少个连接
func (c *CollectConns) ClientsCount() int {
	c.newmutex.Lock()
	defer c.newmutex.Unlock()
	return len(c.clients)
}

// =======================Auth
type AuthCollectConns struct {
	newmutex sync.Mutex
	clients  map[string]*Client
}

// 添加一个客户端
func (c *AuthCollectConns) Add(client *Client) {
	c.newmutex.Lock()
	c.clients[client.phoneNum] = client
	c.newmutex.Unlock()
}

// 删除一个客户端
func (c *AuthCollectConns) Remove(phoneNum string) {
	c.newmutex.Lock()
	if _, ok := c.clients[phoneNum]; ok {
		delete(c.clients, phoneNum)
	}
	c.newmutex.Unlock()
}

// 得到一个key的客户端
func (c *AuthCollectConns) Get(phoneNum string) (*Client, error) {
	c.newmutex.Lock()
	defer c.newmutex.Unlock()
	if _, ok := c.clients[phoneNum]; ok {
		return c.clients[phoneNum], nil
	}
	return nil, errors.New("not exist")
}

// 返回当前有多少个连接
func (c *AuthCollectConns) ClientsCount() int {
	c.newmutex.Lock()
	defer c.newmutex.Unlock()
	return len(c.clients)
}

var Conns CollectConns
var AuthConns AuthCollectConns

func init() {
	//
	AuthConns = AuthCollectConns{clients: make(map[string]*Client)}
	Conns = CollectConns{clients: make(map[string]*Client)}
}

func handle_client(conn net.Conn) {
	client := NewClient(conn)
	fmt.Println("all new Client connect")
	client.Run()
}

func Listen(f func(net.Conn), port int) {
	listen_addr := fmt.Sprintf("0.0.0.0:%d", port)
	listen, err := net.Listen("tcp", listen_addr)
	if err != nil {
		fmt.Println("初始化失败", err.Error())
		return
	}
	fmt.Println("服务器开始运行, 端口", port)
	tcp_listener, ok := listen.(*net.TCPListener)
	if !ok {
		fmt.Println("listen error")
		return
	}

	for {
		client, err := tcp_listener.AcceptTCP()
		if err != nil {

			return
		}
		f(client)
	}
}

func Main() {
	//go crontask.CronSave()
	Listen(handle_client, config.ServerConfig.ElectricIotPort)
}
