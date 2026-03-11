package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
)

type Hub struct {
	clients    map[net.Conn]struct{}
	register   chan net.Conn
	unregister chan net.Conn
	broadcast  chan Message
}

type Message struct {
	sender  net.Conn
	content []byte
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[net.Conn]struct{}),
		register:   make(chan net.Conn),
		unregister: make(chan net.Conn),
		broadcast:  make(chan Message),
	}
}

func (h *Hub) run() {
	for {
		select {
		case conn := <-h.register:
			h.clients[conn] = struct{}{}
		case conn := <-h.unregister:
			delete(h.clients, conn)
		case message := <-h.broadcast:
			for client := range h.clients {
				if client != message.sender {
					_, err := client.Write(message.content)
					if err != nil {
						panic("Could not write to client")
					}
				}
			}
		}
	}
}

func handleConnection(ctx context.Context, conn net.Conn, id int, hub *Hub) {
	defer func() {
		if err := conn.Close(); err != nil {
			panic("Connection could not be closed")
		}
	}()
	hub.register <- conn

	buffer := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done():
			hub.unregister <- conn
			return
		default:
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					hub.unregister <- conn
					fmt.Printf("Client %d disconnected\n", id)
				}
				return
			}
			readBuffer := string(buffer[:n])
			message := fmt.Sprintf("Client %d: %s", id, readBuffer)
			hub.broadcast <- Message{sender: conn, content: []byte(message)}
			fmt.Println(message)
		}
	}
}

func broadcastMessage(hub *Hub) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		hub.broadcast <- Message{sender: nil, content: []byte("Server: " + scanner.Text())}
	}
}

func main() {
	hub := newHub()
	idCount := 0
	listener, err := net.Listen("tcp", ":8080")

	ctx, cancel := context.WithCancel(context.Background())
	if err != nil {
		panic("Could not listen on TCP:8080")
	}

	fmt.Println("Server listening on :8080")

	go broadcastMessage(hub)
	go hub.run()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					fmt.Println("Could not accept connection: ", err)
					continue
				}
			}

			idCount++
			go handleConnection(ctx, conn, idCount, hub)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	fmt.Println("Shutting down server...")
	cancel()

	if err := listener.Close(); err != nil {
		panic("Listener could not be closed")
	}
}
