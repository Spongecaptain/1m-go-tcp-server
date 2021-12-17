package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
)

var (
	// C 为父子进程的总个数
	c       = flag.Int("c", 10, "concurrency")
	prefork = flag.Bool("prefork", false, "use prefork")
	child   = flag.Bool("child", false, "is child proc")
)

/*
说明：实际上，这个项目中的 prefork 借鉴了 apache http server 中父子进程共享 sever socket 的文件描述符的思想
但是 apache prefork 通常不会使用 epoll，其使用 prefork 时，在上世纪 80 年代，甚至 select 都是后来才有的概念
但是这里实际上还是使用了 epoll

如果把进程与 goroutine 看做是一种东西，7_server_prefork 与 6_multiple_sever 其实没有本质上的区别
都是利用多线程 accept、do I/O 搭配 epoll 来实现高并发的 Socket connect 处理
*/
func main() {
	flag.Parse()

	setLimit()

	var ln net.Listener
	var err error
	// 根据 prefork 状态位进行决定是否进行 pre fork
	if *prefork {
		ln = doPrefork(*c)
	} else {
		ln, err = net.Listen("tcp", ":8972")
		if err != nil {
			panic(err)
		}
	}

	startEpoll(ln)

	select {}
}

func startEpoll(ln net.Listener) {
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

// 父、子进程在这个方法的执行上有着不同的逻辑，但实际上返回的 server socket 的文件描述符是同一个（父子进程共享了）
func doPrefork(c int) net.Listener {
	var listener net.Listener
	// 主进程走这个逻辑
	if !*child {
		addr, err := net.ResolveTCPAddr("tcp", ":8972")
		if err != nil {
			log.Fatal(err)
		}
		tcplistener, err := net.ListenTCP("tcp", addr)
		if err != nil {
			log.Fatal(err)
		}
		fl, err := tcplistener.File()
		if err != nil {
			log.Fatal(err)
		}
		children := make([]*exec.Cmd, c)
		for i := range children {
			// 启动的子进程中，会携带 -child 参数，而主进程是不会携带 -child 参数的
			children[i] = exec.Command(os.Args[0], "-prefork", "-child")
			children[i].Stdout = os.Stdout
			children[i].Stderr = os.Stderr
			children[i].ExtraFiles = []*os.File{fl} // 来自父进程的额外参数（文件描述符）自然是 3
			err = children[i].Start()
			if err != nil {
				log.Fatalf("failed to start child: %v", err)
			}
		}
		for _, ch := range children {
			if err := ch.Wait(); err != nil {
				log.Printf("failed to wait child's starting: %v", err)
			}
		}
		os.Exit(0)
	} else {
		// 子进程逻辑
		var err error
		listener, err = net.FileListener(os.NewFile(3, ""))
		if err != nil {
			log.Fatal(err)
		}
	}
	return listener
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
