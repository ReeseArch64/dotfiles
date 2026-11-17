package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// systemctl pode retornar antes de o socket estar pronto na primeira execução.
func waitLaptopSSHPort(port string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		err = probeLaptopSSHPort(port)
		if err == nil {
			return nil
		}
	}
	return err
}

// Verifica o banner SSH sem tentar autenticar. Não exige authorized_keys.
func probeLaptopSSHPort(port string) error {
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return errors.New("porta SSH inválida")
	}
	for _, host := range []string{"127.0.0.1", "::1"} {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 250*time.Millisecond)
		if err != nil {
			continue
		}
		_ = connection.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		reader := bufio.NewReaderSize(connection, 512)
		found := false
		for range 10 {
			line, err := reader.ReadSlice('\n')
			if err != nil {
				break
			}
			if strings.HasPrefix(string(line), "SSH-2.0-") {
				found = true
				break
			}
		}
		connection.Close()
		if found {
			return nil
		}
	}
	return fmt.Errorf("porta %s não respondeu como servidor SSH local", port)
}
