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
		register:   make(chan *User, 1),
		unregister: make(chan *User, 1),
		broadcast:  make(chan Message, 16),
	}
}

// run is the hub's event loop all mutations are serialized through this single goroutine via channels
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

// send attempts to send on ch, but abandons the send if ctx is cancelled.
func send[T any](ctx context.Context, ch chan<- T, val T) bool {
	select {
	case ch <- val:
		return true
	case <-ctx.Done():
		return false
	}
}

func (h *Hub) join(ctx context.Context, user *User) {
	if !send(ctx, h.register, user) {
		return
	}
	msg := fmt.Sprintf("Server: %s connected to room [%s]\n", user.username, h.name)
	fmt.Print(msg)
	send(ctx, h.broadcast, Message{content: []byte(msg)})
}

func (h *Hub) leave(ctx context.Context, user *User) {
	if !send(ctx, h.unregister, user) {
		return
	}
	msg := fmt.Sprintf("Server: %s left room [%s]\n", user.username, h.name)
	fmt.Print(msg)
	send(ctx, h.broadcast, Message{content: []byte(msg)})
}
