# ByteChat

A concurrent, multi-room TCP chat server and client written in Go from scratch -- no frameworks, no third-party networking libraries. ByteChat demonstrates systems-level network programming with raw TCP sockets, idiomatic Go concurrency patterns, and full graceful shutdown semantics.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Features](#features)
- [Concurrency Model](#concurrency-model)
- [Protocol Specification](#protocol-specification)
- [Getting Started](#getting-started)
- [Usage](#usage)
- [Design Decisions](#design-decisions)
- [Dependencies](#dependencies)

## Overview

ByteChat is a real-time, multi-room chat system that operates over raw TCP. The server handles concurrent client connections using a goroutine-per-connection model, organizes clients into named rooms with independent event loops, supports cross-room direct messaging, and shuts down gracefully on OS signals -- all coordinated through Go's concurrency primitives (channels, contexts, mutexes, and WaitGroups).

The entire system is ~450 lines of Go with a single external dependency (`golang.org/x/sync`).

## Architecture

```
                         ┌─────────────────────────────────────────────────┐
                         │                   main()                       │
                         │                                                │
                         │   ┌────────────┐       ┌────────────────────┐  │
                         │   │  TCP Accept │       │   broadcastAll     │  │
                         │   │    Loop     │       │   (stdin → rooms)  │  │
                         │   └──────┬─────┘       └────────────────────┘  │
                         │          │                                      │
                         └──────────┼──────────────────────────────────────┘
                                    │ spawns goroutine per client
                ┌───────────────────┼───────────────────────────┐
                │                   │                           │
        ┌───────▼────────┐  ┌───────▼────────┐  ┌──────────────▼─────┐
        │ handleConn(1)  │  │ handleConn(2)  │  │  handleConn(N)     │
        │ reads commands │  │ reads commands │  │  reads commands    │
        │ dispatches msgs│  │ dispatches msgs│  │  dispatches msgs   │
        └───────┬────────┘  └───────┬────────┘  └──────────┬─────────┘
                │                   │                      │
                │      channel sends: register /           │
                │        unregister / broadcast            │
                ▼                   ▼                      ▼
        ┌────────────────┐  ┌────────────────┐
        │  Hub "general" │  │  Hub "dev"     │    ...  (1 per room)
        │  (event loop)  │  │  (event loop)  │
        │  goroutine     │  │  goroutine     │
        └────────────────┘  └────────────────┘

                    ┌────────────────────────┐
                    │     RoomManager        │
                    │                        │
                    │  rooms map[string]*Hub │  ← protected by sync.RWMutex
                    │  users map[string]*User│  ← protected by sync.RWMutex
                    └────────────────────────┘
```

**Data flow for a chat message:**

1. A client's `handleConnection` goroutine reads bytes from the TCP socket
2. The message is formatted as `[room] username: message` and sent to the room's Hub `broadcast` channel
3. The Hub's single-threaded event loop receives from the channel and writes to every other client's TCP connection

**Data flow for a direct message:**

1. The sender's `handleConnection` goroutine parses the `/dm` command
2. `RoomManager.handleDM` looks up the target user by username (cross-room, via the global `users` map)
3. The message is written directly to the target's TCP connection, bypassing the Hub entirely

## Project Structure

```
bytechat-go/
├── cmd/
│   ├── client/
│   │   └── main.go          # TCP client: bidirectional I/O with errgroup
│   └── server/
│       ├── main.go          # Entrypoint: listener, accept loop, signal handler, graceful shutdown
│       ├── hub.go           # Hub: per-room event loop, channel-based client set management
│       ├── connection.go    # Connection handler: handshake, command dispatch, read loop
│       └── room.go          # RoomManager: room lifecycle, username registry, DMs, server broadcast
├── config/
│   └── config.go            # Shared TCP port constant
├── go.mod
└── go.sum
```

| File | Lines | Responsibility |
|---|---|---|
| `cmd/server/main.go` | 67 | Application entry point. Binds the TCP listener, runs the accept loop (one goroutine per incoming connection), registers a SIGINT/SIGTERM handler, and orchestrates graceful shutdown via context cancellation + WaitGroup. |
| `cmd/server/hub.go` | 71 | Defines the `Hub` type (one per room). Each Hub runs a single-goroutine event loop that processes `register`, `unregister`, and `broadcast` operations via unbuffered channels -- eliminating the need for mutexes on the per-room client set. Failed writes during broadcast trigger automatic dead-client removal. |
| `cmd/server/connection.go` | 89 | Manages individual client connections. Runs the interactive handshake (username selection with uniqueness enforcement, room selection), then enters the main read loop where incoming data is parsed as either a `/dm` command, `/leave` command, or a regular message for broadcast. Includes a context-watcher goroutine that closes the connection to unblock `conn.Read()` during shutdown. |
| `cmd/server/room.go` | 127 | Central coordinator. `RoomManager` holds the global rooms and users maps (protected by `sync.RWMutex`). Implements double-checked locking for lazy room creation, atomic username claiming/releasing, cross-room direct messaging, and a server operator broadcast feature (stdin to all rooms). |
| `cmd/client/main.go` | 89 | Terminal chat client. Uses `errgroup.Group` (from `golang.org/x/sync`) to run two concurrent goroutines -- one reading from the server, one reading from stdin -- with automatic context cancellation when either fails. A third watcher goroutine closes the connection on context cancellation to unblock the other two. |
| `config/config.go` | 4 | Shared configuration constant (TCP port `:8080`) used by both client and server binaries. |

## Features

- **Multi-room chat** -- Users select a room on connect. Rooms are created lazily on first join and support any number of concurrent participants.
- **Room switching** -- The `/leave` command exits the current room and prompts for a new one, all without disconnecting.
- **Cross-room direct messaging** -- `/dm <username> <message>` sends a private message to any connected user regardless of which room they are in.
- **Username uniqueness** -- Usernames are globally unique, enforced by an atomic claim/release mechanism with mutex protection.
- **Server operator broadcast** -- The server operator can type messages into the server's stdin that are broadcast to every room simultaneously.
- **Graceful shutdown** -- SIGINT/SIGTERM triggers coordinated shutdown: context cancellation propagates to all goroutines, the listener is closed, active connections are drained, and `sync.WaitGroup` ensures every goroutine exits before the process terminates.
- **Dead client cleanup** -- Failed writes during Hub broadcast automatically remove unresponsive clients from the room.
- **Zero external networking dependencies** -- Built entirely on Go's `net` standard library package with raw TCP sockets.

## Concurrency Model

ByteChat uses several complementary concurrency patterns:

| Pattern | Where | Why |
|---|---|---|
| **Goroutine-per-connection** | `main.go` accept loop | Each client gets a dedicated goroutine for blocking I/O, the standard Go network server pattern. |
| **Channel-based event loop** | `hub.go` `run()` | Each room's Hub serializes all client set mutations through a single `select` loop over unbuffered channels, avoiding mutexes on the hot path (message fan-out). |
| **`sync.RWMutex`** | `room.go` `RoomManager` | Protects shared maps (`rooms`, `users`) across connection-handler goroutines. Read-locks for lookups, write-locks for mutations. |
| **Double-checked locking** | `room.go` `getOrCreateRoom()` | Optimistic read-lock check, then pessimistic write-lock re-check for room creation -- minimizes contention for the common case (room already exists). |
| **Context cancellation** | Throughout | A root `context.Context` is cancelled on OS signal, propagating shutdown to every goroutine in the system. |
| **`sync.WaitGroup`** | `main.go` | Tracks all spawned goroutines (accept loop, connection handlers, hub event loops, broadcast reader) and blocks process exit until all have completed. |
| **`errgroup.Group`** | `cmd/client/main.go` | Coordinates client goroutines with automatic context cancellation on first error -- if the server disconnects, the input reader stops, and vice versa. |
| **Connection-close unblocking** | `connection.go`, `client/main.go` | Since `net.Conn.Read()` does not support context cancellation, a watcher goroutine closes the connection when the context is cancelled, causing `Read()` to return an error and unblock. |

### Goroutine Inventory (runtime)

For a server with **N** connected clients across **M** rooms:

| Goroutine | Count | Lifetime |
|---|---|---|
| Main (signal handler) | 1 | Process lifetime |
| Accept loop | 1 | Process lifetime |
| Server stdin reader | 1 | Process lifetime |
| Server broadcast dispatcher | 1 | Process lifetime |
| Hub event loop | M | Per room (created lazily) |
| Connection handler | N | Per client connection |
| Connection context watcher | N | Per client connection |
| **Total** | **4 + M + 2N** | |

## Protocol Specification

ByteChat uses a text-based protocol over raw TCP (port 8080). Messages are UTF-8 strings delimited by newlines. The protocol is compatible with `telnet`, `netcat`, or the included Go client.

### Connection Handshake

```
Server → Client:  Enter username: 
Client → Server:  <username>\n
                  (if taken) Server → Client:  Username already taken, please choose another: 
                  (repeat until unique)
Server → Client:  Enter room: 
Client → Server:  <room_name>\n
Server → Room:    Server: <username> connected to room [<room_name>]\n
```

### Client Commands

| Command | Format | Behavior |
|---|---|---|
| Chat message | `<text>` | Broadcast to all other users in the same room as `[room] username: text` |
| Direct message | `/dm <user> <message>` | Private message to any connected user, regardless of room |
| Leave room | `/leave` | Leave current room, server prompts for a new room to join |

### Server Messages

| Message | Trigger |
|---|---|
| `Server: <user> connected to room [<room>]` | User joins a room |
| `Server: <user> left room [<room>]` | User leaves a room |
| `Server: <user> disconnected` | User disconnects |
| `[<room>] <user>: <text>` | Chat message from another user |
| `[DM from <user>]: <text>` | Incoming direct message |
| `[DM to <user>]: <text>` | Outgoing DM confirmation |
| `Server: <text>` | Server operator broadcast (from stdin) |
| `Server: User <user> not found` | DM target does not exist |
| `Server: You cannot DM yourself` | Self-DM attempt |
| `Server: Usage: /dm <username> <message>` | Malformed DM command |

## Getting Started

### Prerequisites

- Go 1.21+ installed ([download](https://go.dev/dl/))

### Build

```bash
# From the bytechat-go directory
go build -o server ./cmd/server
go build -o client ./cmd/client
```

### Run

**Start the server:**

```bash
./server
# Server listening on :8080
```

**Connect with the built-in client:**

```bash
./client
```

**Or connect with netcat/telnet:**

```bash
nc localhost 8080
# or
telnet localhost 8080
```

### Example Session

**Terminal 1 (server):**
```
$ ./server
Server listening on :8080
Server: alice connected to room [general]
Server: bob connected to room [general]
[general] alice: hey bob!
[general] bob: hey alice!
```

**Terminal 2 (alice):**
```
$ ./client
Enter username: alice
Enter room: general
Server: alice connected to room [general]
Server: bob connected to room [general]
hey bob!
[general] bob: hey alice!
/dm bob this is private
[DM to bob]: this is private
/leave
Server: You left [general]. Enter room: dev
Server: alice connected to room [dev]
```

**Terminal 3 (bob):**
```
$ ./client
Enter username: bob
Enter room: general
Server: bob connected to room [general]
[general] alice: hey bob!
hey alice!
[DM from alice]: this is private
Server: alice left room [general]
```

## Design Decisions

**Why raw TCP instead of WebSocket/HTTP?**
The goal was to build a chat system at the transport layer to understand the fundamentals of socket programming, concurrency, and protocol design without framework abstractions. Raw TCP also makes the server compatible with standard Unix tools like `netcat` and `telnet` for testing.

**Why channel-based Hubs instead of mutexes for room state?**
The Hub pattern serializes all room operations (join, leave, broadcast) through a single goroutine's `select` loop. This eliminates mutex contention on the per-room client set during message fan-out -- the hottest path in a chat server. The pattern is adapted from the [Gorilla WebSocket chat example](https://github.com/gorilla/websocket/tree/main/examples/chat), applied to raw TCP.

**Why `sync.RWMutex` for the RoomManager?**
While Hubs use channels for per-room state, the global rooms and users maps are accessed by every connection handler goroutine. An `RWMutex` allows concurrent reads (DM lookups, room existence checks) while serializing writes (room creation, username claim/release). Double-checked locking on room creation avoids taking the write lock in the common case.

**Why `errgroup` in the client?**
The client runs two blocking I/O goroutines (network read, stdin read) that must shut down together. `errgroup.Group` provides exactly the right semantics: when either goroutine returns an error, the context is cancelled, signaling the other to stop. A third watcher goroutine closes the TCP connection to unblock `conn.Read()`, since Go's network reads don't natively support context cancellation.

**Why no persistence?**
ByteChat is a real-time communication system focused on demonstrating concurrent network programming. All state (rooms, users, messages) is in-memory. This keeps the scope focused on the networking and concurrency aspects without introducing database complexity.

## Dependencies

| Package | Version | Purpose |
|---|---|---|
| Go standard library | - | TCP networking (`net`), concurrency (`sync`, `context`), signal handling (`os/signal`), buffered I/O (`bufio`) |
| [`golang.org/x/sync`](https://pkg.go.dev/golang.org/x/sync) | v0.20.0 | `errgroup.Group` for coordinated goroutine lifecycle management in the client |
