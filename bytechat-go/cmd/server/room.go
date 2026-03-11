package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"sync"
)

type User struct {
	conn     net.Conn
	id       int
	username string
}

type RoomManager struct {
	rooms map[string]*Hub
	users map[string]*User
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
	if hub, ok = rm.rooms[name]; ok {
		return hub
	}

	hub = newHub(name)
	wg.Add(1)
	go hub.run(ctx, wg)
	rm.rooms[name] = hub
	return hub
}

func (rm *RoomManager) claimUsername(user *User) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, taken := rm.users[user.username]; taken {
		return false
	}
	rm.users[user.username] = user
	return true
}

func (rm *RoomManager) releaseUsername(username string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.users, username)
}

func (rm *RoomManager) promptUsername(conn net.Conn, buf []byte, id int) (*User, error) {
	for {
		name, err := prompt(conn, "Enter username: ", buf)
		if err != nil {
			return nil, err
		}
		user := &User{conn: conn, id: id, username: name}
		if rm.claimUsername(user) {
			return user, nil
		}
		conn.Write([]byte("Username already taken, please choose another: "))
	}
}

func (rm *RoomManager) handleDM(sender *User, parts []string) {
	if len(parts) < 3 {
		fmt.Fprintf(sender.conn, "Server: Usage: /dm <username> <message>\n")
		return
	}
	targetName, msg := parts[1], parts[2]

	rm.mu.RLock()
	target, ok := rm.users[targetName]
	rm.mu.RUnlock()

	if !ok {
		fmt.Fprintf(sender.conn, "Server: User %s not found\n", targetName)
		return
	}
	fmt.Fprintf(target.conn, "[DM from %s]: %s\n", sender.username, msg)
	fmt.Fprintf(sender.conn, "[DM to %s]: %s\n", targetName, msg)
}

func (rm *RoomManager) broadcastAll(ctx context.Context, wg *sync.WaitGroup) {
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
				hub.broadcast <- Message{content: []byte("Server: " + line + "\n")}
			}
			rm.mu.RUnlock()
		}
	}
}
