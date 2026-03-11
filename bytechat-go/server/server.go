package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

func handleConnection(conn net.Conn, id int, clients *[]net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			panic("Connection could not be closed")
		}
	}()

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
		readBuffer := string(buffer[:n])
		message := fmt.Sprintf("Client %d: %s", id, readBuffer)
		for _, client := range *clients {
			if client != conn {
				_, err := client.Write([]byte(message))
				if err != nil {
					fmt.Println("Client disconnected, can't send")
					return
				}

			}
		}
		fmt.Println(message)
	}
}

func broadcastMessage(clients *[]net.Conn) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		for _, c := range *clients {
			_, err := c.Write([]byte(scanner.Text()))
			if err != nil {
				fmt.Println("Client disconnected, can't send")
				return
			}

		}
	}
}

func main() {
	var clients []net.Conn
	idCount := 0
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic("Could not listen on TCP:8080")
	}

	defer func() {
		if err := listener.Close(); err != nil {
			panic("Listener could not be closed")
		}
	}()

	fmt.Println("Server listening on :8080")

	go broadcastMessage(&clients)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Could not accept connection: ", err)
			continue
		}
		clients = append(clients, conn)

		idCount++
		go handleConnection(conn, idCount, &clients)

	}
}
