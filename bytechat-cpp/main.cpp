#include <cerrno>
#include <cstring>
#include <fcntl.h>
#include <iostream>
#include <netinet/in.h>
#include <string>
#include <sys/epoll.h>
#include <sys/socket.h>
#include <unistd.h>
#include <unordered_map>

enum class SendResult { Success, ClientDisconnected, NetworkError };

// send message in a loop in case it's too long or error occurs
SendResult safe_send(int client_fd, const std::string &message) {
  const char *ptr = message.c_str();
  size_t remaining = message.length();

  while (remaining > 0) {
    // send with MSG_NOSIGNAL
    ssize_t sent = send(client_fd, ptr, remaining, MSG_NOSIGNAL);

    if (sent == -1) {
      if (errno == EINTR)
        continue;
      else if (errno == EPIPE || errno == ECONNRESET) {
        // client disconnected mid sending
        return SendResult::ClientDisconnected;
      }
      return SendResult::NetworkError;
    }
    ptr += sent;
    remaining -= sent;
  }
  return SendResult::Success;
}

struct Connection {
  const int fd;
  std::string buffer;

  explicit Connection(int fd) : fd{fd} {}
};

void set_nonblocking(int fd) {
  int flags = fcntl(fd, F_GETFL, 0);
  fcntl(fd, F_SETFL, flags | O_NONBLOCK);
}

bool handle_read(Connection &conn,
                 std::unordered_map<int, Connection *> &connections) {
  char temp_buffer[1024];
  while (true) {
    ssize_t n = recv(conn.fd, temp_buffer, sizeof(temp_buffer), 0);

    if (n < 0) {
      if (errno == EAGAIN || errno == EWOULDBLOCK) {
        return true;
      }
      return false;
    }
    if (n == 0) {
      return false;
    }

    conn.buffer.append(temp_buffer, n);

    size_t pos;

    while ((pos = conn.buffer.find('\n')) != std::string::npos) {
      std::string msg = conn.buffer.substr(0, pos + 1);
      conn.buffer.erase(0, pos + 1);
      std::cout << "[Client " << conn.fd << "]: " << msg;

      for (auto &[fd, other] : connections) {
        if (fd != conn.fd) {
          safe_send(fd, msg);
        }
      }
    }
  }
}

int safe_receive(Connection &conn) {
  char temp_buffer[1024];

  ssize_t bytes_received = recv(conn.fd, temp_buffer, sizeof(temp_buffer), 0);

  if (bytes_received < 0) {
    std::cerr << "Error receiving the data" << std::endl;
    return -1;
  } else if (bytes_received == 0) {
    std::cout << "Client disconnected" << std::endl;
    return -1;
  }

  conn.buffer.append(temp_buffer, bytes_received);

  size_t pos;
  while ((pos = conn.buffer.find('\n')) != std::string::npos) {
    std::string message = conn.buffer.substr(0, pos);

    std::cout << "Client: " << message << std::endl;
    conn.buffer.erase(0, pos + 1);
  }
  return 0;
}

void broadcast_message(int fd) {
  std::string input;
  if (std::getline(std::cin, input)) {
    std::string message = "Server: " + input;
    safe_send(fd, message.c_str());
  }
}

void remove_client(int epoll_fd, int client_fd,
                   std::unordered_map<int, Connection *> &connections) {
  epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, nullptr);
  close(client_fd);
  delete connections[client_fd];
  connections.erase(client_fd);

  std::cout << "Client disconnected: " << client_fd << "\n";
}

int main() {
  // open tcp ipv4 socket
  int server_fd = socket(AF_INET, SOCK_STREAM, 0);
  if (server_fd < 0) {
    std::cerr << "Could not create socket" << std::endl;
    return 1;
  }

  int opt = 1;
  if (setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt)) < 0) {
    std::cerr << "Could not set SO_REUSEADDR: " << std::strerror(errno)
              << std::endl;
    close(server_fd);
    return 1;
  }

  sockaddr_in addr{};
  addr.sin_family = AF_INET;
  addr.sin_addr.s_addr = INADDR_ANY;
  addr.sin_port = htons(8080);

  // bind socket to port
  if (bind(server_fd, (sockaddr *)&addr, sizeof(addr)) < 0) {
    std::cerr << "Could not bind to port 8080: " << std::strerror(errno)
              << std::endl;
    close(server_fd);
    return 1;
  }

  // mark it passive
  if (listen(server_fd, 10) < 0) {
    std::cerr << "Could not listen on socket\n";
    return 1;
  }

  set_nonblocking(server_fd);

  int epoll_fd = epoll_create1(0);
  if (epoll_fd < 0) {
    std::cerr << "Could not create epoll instance: " << std::strerror(errno)
              << "\n";
    close(server_fd);
    return 1;
  }

  epoll_event ev{};
  ev.events = EPOLLIN;
  ev.data.fd = server_fd;
  epoll_ctl(epoll_fd, EPOLL_CTL_ADD, server_fd, &ev);

  std::unordered_map<int, Connection *> connections;
  epoll_event events[64];

  std::cout << "Server listening on :8080" << std::endl;

  while (true) {
    int n = epoll_wait(epoll_fd, events, 64, -1);

    if (n < 0) {
      if (errno == EINTR)
        continue;
      std::cerr << "Error in epoll_wait: " << std::strerror(errno) << "\n";
      break;
    }

    for (int i = 0; i < n; i++) {
      int fd = events[i].data.fd;

      if (fd == server_fd) {
        // new connection
        int client_fd = accept(server_fd, nullptr, nullptr);
        if (client_fd < 0) {
          std::cerr << "Could not accept connection: " << std::strerror(errno)
                    << "\n";
          continue;
        }

        set_nonblocking(client_fd);

        Connection *conn = new Connection(client_fd);
        connections[client_fd];

        epoll_event client_ev{};
        client_ev.data.fd = client_fd;
        client_ev.events = EPOLLIN;
        client_ev.data.ptr = conn;
        epoll_ctl(epoll_fd, EPOLL_CTL_ADD, client_fd, &ev);

        std::cout << "Client connected: " << client_fd << "\n";
      } else {
        // existing connection has data
        Connection *conn = static_cast<Connection *>(events[i].data.ptr);
        if (!handle_read) {
          remove_client(epoll_fd, conn->fd, connections);
        }
      }
    }
  }

  for (auto &[fd, conn] : connections) {
    delete conn;
  }

  close(server_fd);
  close(epoll_fd);
  return 0;
}
