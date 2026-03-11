package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type Hub struct {
	clients    map[*User]struct{}
	register   chan *User
	unregister chan *User
	broadcast  chan Message
}

type User struct {
	conn     net.Conn
	id       int
	username string
}

type Message struct {
	sender  net.Conn
	content []byte
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*User]struct{}),
		register:   make(chan *User),
		unregister: make(chan *User),
		broadcast:  make(chan Message),
	}
}

func (h *Hub) run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			for client := range h.clients {
				if err := client.conn.Close(); err != nil {
					fmt.Println("Could not close client connection: ", err)
				}
			}
			return

		case conn := <-h.register:
			h.clients[conn] = struct{}{}

		case conn := <-h.unregister:
			delete(h.clients, conn)

		case message := <-h.broadcast:
			for client := range h.clients {
				if client.conn != message.sender {
					_, err := client.conn.Write(message.content)
					if err != nil {
						fmt.Println("Could not send message to client: ", err)
						delete(h.clients, client)
					}
				}
			}
		}
	}
}

func handleConnection(ctx context.Context, conn net.Conn, id int, hub *Hub, wg *sync.WaitGroup) {
	defer wg.Done()

	conn.Write([]byte("Enter username: "))

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Client disconnected before sending username")
		conn.Close()
		return
	}
	username := strings.TrimSpace(string(buffer[:n]))

	user := &User{conn: conn, id: id, username: username}
	hub.register <- user
	fmt.Printf("%s connected\n", user.username)
	hub.broadcast <- Message{sender: nil, content: []byte(fmt.Sprintf("%s connected", user.username))}

	for {
		select {
		case <-ctx.Done():
			hub.unregister <- user
			return

		default:
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					hub.unregister <- user
					fmt.Printf("%s disconnected\n", user.username)
					hub.broadcast <- Message{sender: nil, content: []byte(fmt.Sprintf("%s disconnected", user.username))}
				}
				return
			}
			readBuffer := string(buffer[:n])
			message := fmt.Sprintf("%s: %s", user.username, readBuffer)
			hub.broadcast <- Message{sender: conn, content: []byte(message)}
			fmt.Println(message)
		}
	}
}

func broadcastMessage(ctx context.Context, hub *Hub, wg *sync.WaitGroup) {
	defer wg.Done()
	lines := make(chan string)

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			hub.broadcast <- Message{sender: nil, content: []byte("Server: " + line)}
		}
	}
}

func main() {
	var wg sync.WaitGroup
	hub := newHub()
	idCount := 0
	listener, err := net.Listen("tcp", ":8080")

	ctx, cancel := context.WithCancel(context.Background())
	if err != nil {
		panic("Could not listen on TCP:8080")
	}

	fmt.Println("Server listening on :8080")

	wg.Add(1)
	go broadcastMessage(ctx, hub, &wg)

	wg.Add(1)
	go hub.run(ctx, &wg)

	wg.Add(1)
	go func() {
		defer wg.Done()
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
			wg.Add(1)
			go handleConnection(ctx, conn, idCount, hub, &wg)
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
	wg.Wait()
	fmt.Println("Server gracefully stopped, all goroutines exited")
}
