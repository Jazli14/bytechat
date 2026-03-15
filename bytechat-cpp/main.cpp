#include <iostream>
#include <thread>
#include <string>
#include <sys/socket.h>
#include <netinet/in.h>
#include <cstring>
#include <cerrno>
#include <unistd.h>

enum class SendResult
{
    Success,
    ClientDisconnected,
    NetworkError
};

// send message in a loop in case it's too long or error occurs
SendResult safe_send(int client_fd, const std::string &message)
{
    const char *ptr = message.c_str();
    size_t remaining = message.length();

    while (remaining > 0)
    {
        // send with MSG_NOSIGNAL
        ssize_t sent = send(client_fd, ptr, remaining, MSG_NOSIGNAL);

        if (sent == -1)
        {
            if (errno == EINTR)
                continue;
            else if (errno == EPIPE || errno == ECONNRESET)
            {
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

struct Connection
{
    const int fd;
    std::string buffer;

    explicit Connection(int fd) : fd{fd} {}
};

int safe_receive(Connection &conn)
{
    char temp_buffer[1024];

    ssize_t bytes_received = recv(conn.fd, temp_buffer, sizeof(temp_buffer), 0);

    if (bytes_received < 0)
    {
        std::cerr << "Error receiving the data" << std::endl;
        return -1;
    }
    else if (bytes_received == 0)
    {
        std::cout << "Client disconnected" << std::endl;
        return -1;
    }

    conn.buffer.append(temp_buffer, bytes_received);

    size_t pos;
    while ((pos = conn.buffer.find('\n')) != std::string::npos)
    {
        std::string message = conn.buffer.substr(0, pos);
        std::cout << "Client: " << message << std::endl;
        conn.buffer.erase(0, pos + 1);
    }
    return 0;
}

void broadcast_message(int fd)
{
    std::string input;
    if (std::getline(std::cin, input))
    {
        std::string message = "Server: " + input;
        safe_send(fd, message.c_str());
    }
}

int main()
{
    // open tcp ipv4 socket
    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (server_fd < 0)
    {
        std::cerr << "Could not create socket" << std::endl;
        return 1;
    }

    int opt = 1;
    if (setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt)) < 0)
    {
        std::cerr << "Could not set SO_REUSEADDR: " << std::strerror(errno) << std::endl;
        close(server_fd);
        return 1;
    }

    sockaddr_in addr{};
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    // load in port
    addr.sin_port = htons(8080);

    // bind socke to port
    if (bind(server_fd, (sockaddr *)&addr, sizeof(addr)) < 0)
    {
        std::cerr << "Could not bind to port 8080: " << std::strerror(errno) << std::endl;
        close(server_fd);
        return 1;
    }

    // mark it passive
    if (listen(server_fd, 10) < 0)
    {
        std::cerr << "Could not listen on socket" << std::endl;
        return 1;
    }
    std::cout << "Server listening on :8080" << std::endl;

    // block until a connection happens
    int client_fd = accept(server_fd, nullptr, nullptr);
    if (client_fd < 0)
    {
        std::cerr << "Could not accept connection" << std::endl;
    }

    std::cout << "Client connected" << std::endl;
    Connection conn(client_fd);

    std::thread(broadcast_message, conn.fd).detach();

    while (true)
    {
        if (safe_receive(conn) < 0)
        {
            break;
        }
    }

    close(client_fd);
    close(server_fd);
    return 0;
}
