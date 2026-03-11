package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

func receiveMessages(conn net.Conn) {
	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Client disconnected")
			} else {
				fmt.Println("Error reading from connection: ", err)
			}
			return
		}
		fmt.Println(string(buffer[:n]))
	}
}

func main() {
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		panic("Failed to dial into TCP address :8080")
	}

	defer func() {
		if err := conn.Close(); err != nil {
			panic("Failed to close connection")
		}
	}()

	go receiveMessages(conn)

	fmt.Println("Connected to TCP :8080")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		_, err := conn.Write([]byte(scanner.Text()))
		if err != nil {
			panic("Failed to write to connection")
		}
	}
}
