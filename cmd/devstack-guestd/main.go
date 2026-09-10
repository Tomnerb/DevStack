//go:build linux

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/mdlayher/vsock"
)

func main() {
	port := flag.Uint(
		"port",
		10250,
		"virtio-vsock listening port",
	)

	target := flag.String(
		"target",
		"/run/containerd/containerd.sock",
		"Unix socket to proxy",
	)

	flag.Parse()

	listener, err := vsock.Listen(
		uint32(*port),
		nil,
	)
	if err != nil {
		log.Fatalf(
			"listen vsock %d: %v",
			*port,
			err,
		)
	}
	defer listener.Close()

	log.Printf(
		"devstack-guestd: vsock:%d -> unix:%s",
		*port,
		*target,
	)

	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		go proxy(
			connection,
			*target,
		)
	}
}

func proxy(
	source net.Conn,
	target string,
) {
	defer source.Close()

	destination, err := net.DialTimeout(
		"unix",
		target,
		3*time.Second,
	)
	if err != nil {
		_, _ = fmt.Fprintf(
			os.Stderr,
			"devstack-guestd: dial %s: %v\n",
			target,
			err,
		)
		return
	}
	defer destination.Close()

	done := make(chan struct{}, 2)

	go func() {
		_, _ = io.Copy(
			destination,
			source,
		)
		done <- struct{}{}
	}()

	go func() {
		_, _ = io.Copy(
			source,
			destination,
		)
		done <- struct{}{}
	}()

	<-done
}
