# ByteChat

A terminal-based TCP chat application built twice, once in **Go** and once in **C++** as a language learning project for systems programming, networking, and concurrency.

## Purpose

The goal is not to build a production chat app, but to implement the same system in two languages and compare how each one approaches the same problems. Go and C++ sit at opposite ends of the systems programming spectrum Go trades control for simplicity with goroutines and channels, while C++ demands more discipline around memory and synchronization but gives you full control.

## Implementations

| | Language | Status |
|---|---|---|
| [`bytechat-go`](./bytechat-go) | Go | Complete |
| [`bytechat-cpp`](./bytechat-cpp) | C++ | In progress |

Each implementation has its own README with architecture details, design decisions, and build instructions.

## Feature Parity Target

Both implementations aim to support the same feature set:

- Raw TCP client/server architecture
- Multiple concurrent clients
- Named chat rooms created lazily on first join
- Room switching without disconnecting (`/leave`)
- Globally unique usernames
- Cross-room direct messaging (`/dm <username> <message>`)
- Server operator broadcast (stdin → all rooms)
- Graceful shutdown on SIGINT/SIGTERM

## Protocol

Both implementations use the same text-based protocol over TCP (port 8080), compatible with `netcat` and `telnet`.

### Handshake
```
Server → Client:  Enter username:
Client → Server:  <username>
Server → Client:  Enter room:
Client → Server:  <room_name>
```

### Commands

| Command | Description |
|---|---|
| `<message>` | Broadcast to current room |
| `/dm <username> <message>` | Direct message any connected user |
| `/leave` | Leave current room and join another |

## Why Both Languages?

Building the same system twice makes the tradeoffs concrete:

- **Go**: goroutines, channels, the hub pattern, context cancellation, errgroup
- **C++**: POSIX sockets, `std::thread`, `std::mutex`, condition variables, RAII

Same problem, different tools. The comparison is the point.
