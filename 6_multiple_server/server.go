package main

import (
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"syscall"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/rcrowley/go-metrics"
)

var (
	// C 是
	c = flag.Int("c", 10, "concurrency")
)

var (
	opsRate = metrics.NewRegisteredMeter("ops", nil)
)

/*
 这里相当于开了 c 对 goroutine，共 2*c 个 goroutine
 对于每对 goroutine，有：
 第一个 goroutine 负责监听 server socket，accept 新连接，注册到 epoll 中
 第二个 goroutine 负责 epoll_wait，然后执行 I/O logic 逻辑

 由于 Go 中的 goroutine 比较轻量，开有限个 goroutine 的开销总不会很大（有限个指的是不会随着 connection 增多而需要提高 goroutine 个数）
*/

func main() {
	flag.Parse()

	setLimit()
	go metrics.Log(metrics.DefaultRegistry, 5*time.Second, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))

	go func() {
		if err := http.ListenAndServe(":6060", nil); err != nil {
			log.Fatalf("pprof failed: %v", err)
		}
	}()

	for i := 0; i < *c; i++ {
		go startEpoll()
	}

	select {}
}

func startEpoll() {
	// 使用 reuseport 库启动多个 goroutine 监听同一个端口，这个特性在较新的 Linux 内核上已经支持，内核会负责负载均衡
	ln, err := reuseport.Listen("tcp", ":8972")
	if err != nil {
		panic(err)
	}

	epoller, err := MkEpoll()
	if err != nil {
		panic(err)
	}

	go start(epoller)

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

func start(epoller *epoll) {
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
			io.CopyN(conn, conn, 8)
			if err != nil {
				if err := epoller.Remove(conn); err != nil {
					log.Printf("failed to remove %v", err)
				}
				conn.Close()
			}

			opsRate.Mark(1)
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
