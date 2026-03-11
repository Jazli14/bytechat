package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/sync/errgroup"
)

var errInputClosed = errors.New("input channel closed")

func receiveMessages(ctx context.Context, conn net.Conn) error {
	buffer := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			n, err := conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					return fmt.Errorf("server disconnected")
				} else {
					return fmt.Errorf("error reading from connection: %w", err)
				}
			}
			fmt.Println(string(buffer[:n]))
		}
	}
}

func readInput(ctx context.Context, conn net.Conn) error {
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
			return nil
		case line, ok := <-lines:
			if !ok {
				return errInputClosed
			}
			_, err := conn.Write([]byte(line))
			if err != nil {
				return fmt.Errorf("could not send message: %w", err)
			}
		}
	}
}

func main() {
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		panic("Failed to dial into TCP address :8080")
	}

	defer conn.Close()

	g, ctx := errgroup.WithContext(context.Background())

	g.Go(func() error { return receiveMessages(ctx, conn) })
	g.Go(func() error { return readInput(ctx, conn) })
	g.Go(func() error {
		<-ctx.Done()
		conn.Close()
		return nil
	})

	if err := g.Wait(); err != nil && !errors.Is(err, errInputClosed) {
		fmt.Println(err)
	}
}
