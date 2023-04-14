package main

import (
	"context"
	"net"
	"strings"
	"syscall"
)

type multiListener struct {
	listeners []*net.TCPListener
	connChan  chan acceptResult
	ctx       context.Context
	cancel    context.CancelFunc
}

type acceptResult struct {
	conn net.Conn
	err  error
}

func newListener(addr string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	iPort, err := net.LookupPort("tcp", port)
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	if host == "" {
		return net.Listen("tcp", addr)
	} else if host == "localhost" {
		ips, err = getLocalhostIp()
		if err != nil {
			return nil, err
		}
	} else {
		ips, err = net.LookupIP(host)
		if err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return net.Listen("tcp", addr)
	} else if len(ips) == 1 {
		return net.ListenTCP("tcp", &net.TCPAddr{IP: ips[0], Port: iPort})
	}
	return newMultiListener(ips, iPort)
}

func getLocalhostIp() ([]net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	rt := make([]net.IP, 0, 2)
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && ip.IP.IsLoopback() {
			rt = append(rt, ip.IP)
		}
	}
	return rt, nil
}

func newMultiListener(ips []net.IP, port int) (lis net.Listener, err error) {
	listeners := make([]*net.TCPListener, 0, len(ips))
	defer func() {
		if err != nil {
			for _, l := range listeners {
				l.Close()
			}
		}
	}()
	for _, ip := range ips {
		l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: ip, Port: port})
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt := &multiListener{
		listeners: listeners,
		connChan:  make(chan acceptResult),
		ctx:       ctx,
		cancel:    cancel,
	}
	for _, l := range listeners {
		l := l
		go func() {
			for {
				conn, err := l.AcceptTCP()
				select {
				case rt.connChan <- acceptResult{conn: conn, err: err}:
				case <-ctx.Done():
					return
				}

			}
		}()
	}

	return rt, nil
}

func (ml *multiListener) Accept() (net.Conn, error) {
	select {
	case ar := <-ml.connChan:
		return ar.conn, ar.err
	case <-ml.ctx.Done():
		return nil, syscall.EINVAL
	}
}

func (ml *multiListener) Close() error {
	ml.cancel()
	for _, l := range ml.listeners {
		l.Close()
	}
	return nil
}

func (ml *multiListener) Addr() net.Addr {
	return ml.listeners[0].Addr()
}

func printListener(lis net.Listener) string {
	switch l := lis.(type) {
	case *multiListener:
		addrs := make([]string, 0, len(l.listeners))
		for _, l := range l.listeners {
			addrs = append(addrs, l.Addr().String())
		}
		return strings.Join(addrs, ",")
	default:
		return lis.Addr().String()
	}
}
