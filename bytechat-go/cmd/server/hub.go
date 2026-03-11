package main

import (
	"context"
	"fmt"
	"net"
	"sync"
)

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

// run is the hub's event loop
func (h *Hub) run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case user := <-h.register:
			h.clients[user] = struct{}{}
		case user := <-h.unregister:
			delete(h.clients, user)
		case msg := <-h.broadcast:
			for client := range h.clients {
				if client.conn != msg.sender {
					if _, err := client.conn.Write(msg.content); err != nil {
						fmt.Println("Could not send message to client:", err)
						delete(h.clients, client)
					}
				}
			}
		}
	}
}

func (h *Hub) join(user *User) {
	h.register <- user
	msg := fmt.Sprintf("Server: %s connected to room [%s]\n", user.username, h.name)
	fmt.Print(msg)
	h.broadcast <- Message{content: []byte(msg)}
}

func (h *Hub) leave(user *User) {
	h.unregister <- user
	msg := fmt.Sprintf("Server: %s left room [%s]\n", user.username, h.name)
	fmt.Print(msg)
	h.broadcast <- Message{content: []byte(msg)}
}
