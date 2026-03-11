package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

func prompt(conn net.Conn, msg string, buf []byte) (string, error) {
	if _, err := conn.Write([]byte(msg)); err != nil {
		return "", err
	}
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf[:n])), nil
}

func handleConnection(ctx context.Context, conn net.Conn, rm *RoomManager, wg *sync.WaitGroup) {
	defer wg.Done()
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 1024)

	user, err := rm.promptUsername(conn, buf)
	if err != nil {
		fmt.Println("Client disconnected before sending username")
		return
	}
	defer rm.releaseUsername(user.username)

	roomName, err := promptNonEmpty(conn, "Enter room: ", buf)
	if err != nil {
		fmt.Printf("Server: %s disconnected before sending room name\n", user.username)
		return
	}

	hub := rm.getOrCreateRoom(ctx, wg, roomName)
	hub.join(ctx, user)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := conn.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Printf("Server: %s disconnected\n", user.username)
					hub.leave(ctx, user)
				}
				return
			}

			line := strings.TrimSpace(string(buf[:n]))
			if line == "" {
				continue
			}

			switch {
			case strings.HasPrefix(line, "/dm "):
				parts := strings.SplitN(line, " ", 3)
				rm.handleDM(user, parts)

			case line == "/leave":
				hub.leave(ctx, user)
				roomName, err := promptNonEmpty(conn, fmt.Sprintf("Server: You left [%s]. Enter room: ", hub.name), buf)
				if err != nil {
					fmt.Printf("Server: %s disconnected before sending new room name\n", user.username)
					return
				}
				hub = rm.getOrCreateRoom(ctx, wg, roomName)
				hub.join(ctx, user)

			default:
				msg := fmt.Sprintf("[%s] %s: %s\n", hub.name, user.username, line)
				send(ctx, hub.broadcast, Message{sender: conn, content: []byte(msg)})
				fmt.Print(msg)
			}
		}
	}
}

// promptNonEmpty repeats the prompt until the user provides a non-empty value.
func promptNonEmpty(conn net.Conn, msg string, buf []byte) (string, error) {
	for {
		val, err := prompt(conn, msg, buf)
		if err != nil {
			return "", err
		}
		if val != "" {
			return val, nil
		}
		msg = "Input cannot be empty. " + msg
	}
}
