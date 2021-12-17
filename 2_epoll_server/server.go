package main

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"syscall"
)

var epoller *epoll

/*
以下是伪代码，只有两个 goroutine

main goroutine:

1. epoll_create
2. for{
	net.Listener.Accept
	epoll_ctrl
	}

child goroutine:

for{
	epoll_wait
	do I/O logic
}

*/
func main() {
	setLimit()
	// 1. create server socket to listen a port
	ln, err := net.Listen("tcp", ":8972")
	if err != nil {
		panic(err)
	}

	go func() {
		if err := http.ListenAndServe(":6060", nil); err != nil {
			log.Fatalf("pprof failed: %v", err)
		}
	}()
	// 2. epoll create
	epoller, err = MkEpoll()
	if err != nil {
		panic(err)
	}
	// 3. 启动一个 goroutine，负责调用 epoll_wait 方法，然后处理其上的 I/O 数据
	go start()
	// 4. for 循环中不断接受新连接，然后将新连接对应的文件描述符注册到 epoll 中，指定对相关事件感兴趣
	for {
		conn, e := ln.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				log.Printf("accept temp err: %v", ne)
				continue
			}

			log.Printf("accept err: %v", e)
			return
		}

		if err := epoller.Add(conn); err != nil {
			log.Printf("failed to add connection %v", err)
			conn.Close()
		}
	}
}

func start() {
	var buf = make([]byte, 8)
	for {
		connections, err := epoller.Wait()
		if err != nil {
			log.Printf("failed to epoll wait %v", err)
			continue
		}
		for _, conn := range connections {
			if conn == nil {
				break
			}
			if _, err := conn.Read(buf); err != nil {
				if err := epoller.Remove(conn); err != nil {
					log.Printf("failed to remove %v", err)
				}
				conn.Close()
			}
		}
	}
}

func setLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}

	log.Printf("set cur limit: %d", rLimit.Cur)
}
