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

	"github.com/Jazli14/bytechat/bytechat-go/config"
)

type User struct {
	conn     net.Conn
	id       int
	username string
}

type Message struct {
	sender  net.Conn
	content []byte
}

type Hub struct {
	name       string
	clients    map[*User]struct{}
	register   chan *User
	unregister chan *User
	broadcast  chan Message
}

func newHub(name string) *Hub {
	return &Hub{
		name:       name,
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

type RoomManager struct {
	rooms map[string]*Hub
	mu    sync.RWMutex
}

func (rm *RoomManager) getOrCreateRoom(ctx context.Context, wg *sync.WaitGroup, name string) *Hub {
	rm.mu.RLock()
	hub, ok := rm.rooms[name]
	rm.mu.RUnlock()
	if ok {
		return hub
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()
	if hub, ok := rm.rooms[name]; ok {
		return hub
	}

	hub = newHub(name)

	wg.Add(1)
	go hub.run(ctx, wg)
	rm.rooms[name] = hub
	return hub
}

func handleConnection(ctx context.Context, conn net.Conn, id int, rm *RoomManager, wg *sync.WaitGroup) {
	defer wg.Done()
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	conn.Write([]byte("Enter username: "))

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Client disconnected before sending username")
		return
	}
	username := strings.TrimSpace(string(buffer[:n]))

	user := &User{conn: conn, id: id, username: username}

	conn.Write([]byte("Enter room: "))
	n, err = conn.Read(buffer)
	if err != nil {
		fmt.Printf("Server: %s disconnected before sending room name\n", user.username)
		return
	}
	roomName := strings.TrimSpace(string(buffer[:n]))
	hub := rm.getOrCreateRoom(ctx, wg, roomName)
	hub.register <- user

	connectionMsg := fmt.Sprintf("Server: %s connected to room [%s]\n", user.username, hub.name)
	fmt.Printf("%s", connectionMsg)
	hub.broadcast <- Message{sender: nil, content: []byte(connectionMsg)}

	for {
		select {
		case <-ctx.Done():
			return

		default:
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					hub.unregister <- user

					disconnectionMsg := fmt.Sprintf("Server: %s disconnected from the server\n", user.username)
					fmt.Printf("%s", disconnectionMsg)
					hub.broadcast <- Message{sender: nil, content: []byte(disconnectionMsg)}
				}
				return
			}
			readBuffer := string(buffer[:n])
			if strings.TrimSpace(readBuffer) == "/leave" {
				hub.unregister <- user
				hub.broadcast <- Message{sender: nil, content: []byte(fmt.Sprintf("Server: %s left the room [%s]", user.username, hub.name))}
				fmt.Printf("Server: %s left room [%s]\n", user.username, hub.name)

				fmt.Fprintf(conn, "Server: You have left the room [%s]\nEnter room: ", hub.name)
				n, err = conn.Read(buffer)
				if err != nil {
					fmt.Printf("Server: %s disconnected before sending new room name\n", user.username)
					return
				}
				roomName := strings.TrimSpace(string(buffer[:n]))
				hub = rm.getOrCreateRoom(ctx, wg, roomName)
				hub.register <- user

				connectionMsg := fmt.Sprintf("Server: %s connected to room [%s]\n", user.username, hub.name)
				fmt.Printf("%s", connectionMsg)
				hub.broadcast <- Message{sender: nil, content: []byte(connectionMsg)}
				continue
			}

			message := fmt.Sprintf("[%s] %s: %s", hub.name, user.username, readBuffer)
			hub.broadcast <- Message{sender: conn, content: []byte(message)}
			fmt.Println(message)
		}
	}
}

func broadcastMessage(ctx context.Context, rm *RoomManager, wg *sync.WaitGroup) {
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
			rm.mu.RLock()
			for _, hub := range rm.rooms {
				hub.broadcast <- Message{sender: nil, content: []byte("Server: " + line)}
			}
			rm.mu.RUnlock()
		}
	}
}

func main() {
	var wg sync.WaitGroup
	rm := &RoomManager{rooms: make(map[string]*Hub)}
	idCount := 0
	listener, err := net.Listen("tcp", config.Port)

	ctx, cancel := context.WithCancel(context.Background())
	if err != nil {
		panic("Could not listen on TCP" + config.Port)
	}

	fmt.Println("Server listening on " + config.Port)

	wg.Add(1)
	go broadcastMessage(ctx, rm, &wg)

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
					fmt.Println("Server: Could not accept connection: ", err)
					continue
				}
			}

			idCount++
			wg.Add(1)
			go handleConnection(ctx, conn, idCount, rm, &wg)
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
