package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"
)

// _ go flag 用于方便处理命令行参数
var (
	ip          = flag.String("ip", "127.0.0.1", "server IP")
	connections = flag.Int("conn", 1, "number of tcp connections")

	startMetric = flag.String("sm", time.Now().Format("2006-01-02T15:04:05 -0700"), "start time point of all clients") // never used
)

func main() {
	flag.Parse()

	setLimit()

	addr := *ip + ":8972"
	log.Printf("连接到 %s", addr)
	var conns []net.Conn
	for i := 0; i < *connections; i++ {
		c, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			fmt.Println("failed to connect", i, err)
			i-- // 连接失败，我们需要继续连接，目标是最后连接数就是完完整整的 connections 个，而不是仅仅是尝试 connections 次连接
			continue
		}
		conns = append(conns, c)
		time.Sleep(time.Millisecond)
	}

	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	log.Printf("完成初始化 %d 连接", len(conns))

	tts := time.Second
	if *connections > 100 {
		tts = time.Millisecond * 5
	}
	// 以 tts 为间隔，依次向多个连接发送指定的数据
	for {
		for i := 0; i < len(conns); i++ {
			time.Sleep(tts)
			conn := conns[i]
			//log.Printf("连接 %d 发送数据", i)
			conn.Write([]byte("hello world\r\n"))
		}
	}
}

// Rlimit 是 Linux 内核控制 用户 或 进程 资源占用的机制
// 控制的内容包括：内存、文件、锁、CPU 调度、进程数等
// RLIMIT_NOFILE 的含义是：进程打开的文件描述符
// setLimit 执行语义：将进程可打开文件描述符配置到最大
func setLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
}
