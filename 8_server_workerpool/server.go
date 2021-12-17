package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"syscall"
	"time"

	"github.com/rcrowley/go-metrics"
)

var (
	c = flag.Int("c", 10, "concurrency")
)
var (
	opsRate = metrics.NewRegisteredMeter("ops", nil)
)

var epoller *epoll
var workerPool *pool

/*
这里的协程模型是：
	大小为 c 的协程池来负责执行所有业务逻辑，业务逻辑非常简单，就是简单的 echo
	main goroutine 来负责 server socket 上的新连接监听，并负责将其注册到 epoll 中
	一个 child goroutine 负责执行 epoll_wait，然后将 Socket 连接交给线程池处理

	思想应当是去耦合化：
	这里 main goroutine 以及 child goroutine 都可以被称为 I/O goroutine，前者负责新连接事件，后者负责读写事件
	这里的设计并非符合模板设计，因为数据的反序列化也属于 I/O 逻辑的一部分，我们不应该将二进制数据的反序列化交给业务线程池负责
	child goroutine 最好的方式是反序列化为一个消息交给协程池来处理

	TODO 8_server_workerpool 与 6_multiple_server 相比，实际上区别不大
	在这里线程池的意义并不会很大，不过 为什么会有延迟上的区别呢？
	TODO 有待进一步测试。测试环境中的 协程数（进程数）都是 50 么？
*/

func main() {
	flag.Parse()

	setLimit()
	go metrics.Log(metrics.DefaultRegistry, 5*time.Second, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))

	ln, err := net.Listen("tcp", ":8972")
	if err != nil {
		panic(err)
	}

	go func() {
		if err := http.ListenAndServe(":6060", nil); err != nil {
			log.Fatalf("pprof failed: %v", err)
		}
	}()

	workerPool = newPool(*c, 1000000)
	workerPool.start()

	epoller, err = MkEpoll()
	if err != nil {
		panic(err)
	}

	go start()

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

	workerPool.Close()
}

func start() {
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

			workerPool.addTask(conn)
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
