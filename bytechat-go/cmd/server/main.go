package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Jazli14/bytechat/bytechat-go/config"
)

func main() {
	var wg sync.WaitGroup
	rm := &RoomManager{
		rooms: make(map[string]*Hub),
		users: make(map[string]*User),
	}

	listener, err := net.Listen("tcp", config.Port)
	if err != nil {
		fmt.Println("Could not listen on TCP" + config.Port + ": ", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	fmt.Println("Server listening on " + config.Port)

	wg.Add(1)
	go rm.broadcastAll(ctx, &wg)

	// accept loop spawns a handleConnection goroutine per client
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
					fmt.Println("Server: Could not accept connection:", err)
					continue
				}
			}
			wg.Add(1)
			go handleConnection(ctx, conn, rm, &wg)
		}
	}()

	// block until SIGINT/SIGTERM, then cancel ctx and close the listener
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
