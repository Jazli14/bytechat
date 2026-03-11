package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

// prompt writes msg to conn and returns the trimmed response.
func prompt(conn net.Conn, msg string, buf []byte) (string, error) {
	conn.Write([]byte(msg))
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf[:n])), nil
}

func handleConnection(ctx context.Context, conn net.Conn, id int, rm *RoomManager, wg *sync.WaitGroup) {
	defer wg.Done()
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 1024)

	user, err := rm.promptUsername(conn, buf, id)
	if err != nil {
		fmt.Println("Client disconnected before sending username")
		return
	}
	defer rm.releaseUsername(user.username)

	roomName, err := prompt(conn, "Enter room: ", buf)
	if err != nil {
		fmt.Printf("Server: %s disconnected before sending room name\n", user.username)
		return
	}

	hub := rm.getOrCreateRoom(ctx, wg, roomName)
	hub.join(user)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := conn.Read(buf)
			if err != nil {
				if err == io.EOF {
					msg := fmt.Sprintf("Server: %s disconnected\n", user.username)
					fmt.Print(msg)
					hub.unregister <- user
					hub.broadcast <- Message{content: []byte(msg)}
				}
				return
			}

			line := strings.TrimSpace(string(buf[:n]))

			switch {
			case strings.HasPrefix(line, "/dm "):
				parts := strings.SplitN(line, " ", 3)
				rm.handleDM(user, parts)

			case line == "/leave":
				hub.leave(user)
				roomName, err := prompt(conn, fmt.Sprintf("Server: You left [%s]. Enter room: ", hub.name), buf)
				if err != nil {
					fmt.Printf("Server: %s disconnected before sending new room name\n", user.username)
					return
				}
				hub = rm.getOrCreateRoom(ctx, wg, roomName)
				hub.join(user)

			default:
				msg := fmt.Sprintf("[%s] %s: %s\n", hub.name, user.username, line)
				hub.broadcast <- Message{sender: conn, content: []byte(msg)}
				fmt.Print(msg)
			}
		}
	}
}
