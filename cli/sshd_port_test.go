package main

import (
	"fmt"
	"net"
	"testing"
)

func TestProbeSSHPort(t *testing.T) {
	for _, banner := range []string{"SSH-2.0-OpenSSH_test\r\n", "HTTP/1.1 200 OK\r\n"} {
		t.Run(banner[:3], func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				connection, err := listener.Accept()
				if err == nil {
					fmt.Fprint(connection, banner)
					connection.Close()
				}
			}()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			err = probeSSHPort(port)
			if (err == nil) != (banner[:3] == "SSH") {
				t.Fatalf("banner %q: %v", banner, err)
			}
			<-done
		})
	}
	for _, invalid := range []string{"", "0", "65536", "ssh"} {
		if err := probeSSHPort(invalid); err == nil {
			t.Fatalf("porta inválida aceita: %s", invalid)
		}
	}
}
