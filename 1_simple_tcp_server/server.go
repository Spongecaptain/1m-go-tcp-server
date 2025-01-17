package main

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"syscall"
)

func main() {
	// setLimit()

	ln, err := net.Listen("tcp", ":8972")
	if err != nil {
		panic(err)
	}

	go func() {
		if err := http.ListenAndServe(":6060", nil); err != nil {
			log.Fatalf("pprof failed: %v", err)
		}
	}()

	var connections []net.Conn
	// 这里的处理是特殊的，一般而言连接会尽早关闭，例如使用一个心跳机制来将 idle 连接关闭，而不是这里在 server 应用程序退出时关闭
	// 这里是为了查看百万连接的内存、CPU 占用，才这样做的
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()

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

		go handleConn(conn)
		connections = append(connections, conn)
		if len(connections)%100 == 0 {
			log.Printf("total number of connections: %v", len(connections))
		}
	}
}

// Discard 是一个 io.Writer 接口，调用它的 Write 方法将不做任何事情
// 相当于一个 io 逻辑的 mock 操作
// 连 echo 的步骤都省略了
func handleConn(conn net.Conn) {
	io.Copy(ioutil.Discard, conn)
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
