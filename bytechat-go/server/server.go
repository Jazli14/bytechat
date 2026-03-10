package main

import (
	"fmt"
	"net"
)

func handleConnection(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			panic("Connection could not be closed")
		}
	}()
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			panic("Could not read from connection")
		}

		fmt.Println(string(buffer[:n]))
	}
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic("Could not listen on TCP:8080")
	}

	defer func() {
		if err := listener.Close(); err != nil {
			panic("Listener could not be closed")

		}
	}()

	fmt.Println("server listening on :8080")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Could not accept connection: ", err)
			continue
		}

		go handleConnection(conn)

	}
}
