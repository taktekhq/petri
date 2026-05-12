package proxy

import (
	"io"
	"net"
)

// bridge copies bytes between a and b in both directions, returning once
// either side has stopped sending.
func bridge(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go forward(a, b, done)
	go forward(b, a, done)
	<-done
	<-done
}

// forward copies src→dst, then half-closes dst so the matching forward in the
// other direction unblocks and returns.
func forward(dst, src net.Conn, done chan<- struct{}) {
	io.Copy(dst, src)
	dst.Close()
	done <- struct{}{}
}
