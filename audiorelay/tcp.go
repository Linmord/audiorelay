package audiorelay

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// TCPServer handles TCP client connections and data broadcasting
type TCPServer struct {
	config    *Config
	listener  net.Listener
	clients   map[net.Conn]bool
	clientsMu sync.RWMutex

	// Control
	isRunning bool
}

// NewTCPServer creates a new TCP server instance
func NewTCPServer(config *Config) *TCPServer {
	return &TCPServer{
		config:  config,
		clients: make(map[net.Conn]bool),
	}
}

// Start begins the TCP server
func (ts *TCPServer) Start() error {
	var err error
	ts.listener, err = net.Listen("tcp", ":"+ts.config.Server.Port)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %v", err)
	}

	ts.isRunning = true

	// Display server information
	ts.displayServerInfo()

	// Start accepting clients
	go ts.acceptClients()

	return nil
}

// Stop gracefully shuts down the TCP server
func (ts *TCPServer) Stop() {
	ts.isRunning = false

	if ts.listener != nil {
		ts.listener.Close()
	}

	// Close all client connections
	ts.clientsMu.Lock()
	for client := range ts.clients {
		client.Close()
	}
	ts.clients = make(map[net.Conn]bool)
	ts.clientsMu.Unlock()

	fmt.Println(" TCP server stopped")
}

// Broadcast sends audio data to all connected clients
func (ts *TCPServer) Broadcast(data []byte) {
	ts.clientsMu.RLock()
	defer ts.clientsMu.RUnlock()

	if len(ts.clients) == 0 {
		return
	}

	failedClients := make([]net.Conn, 0)

	for client := range ts.clients {
		client.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := client.Write(data)
		if err != nil {
			failedClients = append(failedClients, client)
		}
	}

	// Clean up failed clients
	if len(failedClients) > 0 {
		go ts.cleanupClients(failedClients)
	}
}

// GetClientCount returns the number of connected clients
func (ts *TCPServer) GetClientCount() int {
	ts.clientsMu.RLock()
	defer ts.clientsMu.RUnlock()
	return len(ts.clients)
}

// acceptClients handles incoming client connections
func (ts *TCPServer) acceptClients() {
	for ts.isRunning {
		conn, err := ts.listener.Accept()
		if err != nil {
			if ts.isRunning {
				log.Printf("Client connection error: %v", err)
			}
			return
		}

		// Optimize TCP connection
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetNoDelay(true)
			tcpConn.SetWriteBuffer(32 * 1024)
			tcpConn.SetReadBuffer(16 * 1024)
			tcpConn.SetKeepAlive(true)
		}

		fmt.Printf(" Client connected: %s\n", conn.RemoteAddr())
		ts.addClient(conn)
	}
}

// addClient adds a new client to the connection pool
func (ts *TCPServer) addClient(conn net.Conn) {
	ts.clientsMu.Lock()
	defer ts.clientsMu.Unlock()
	ts.clients[conn] = true
}

// cleanupClients removes failed client connections
func (ts *TCPServer) cleanupClients(failedClients []net.Conn) {
	ts.clientsMu.Lock()
	defer ts.clientsMu.Unlock()

	for _, client := range failedClients {
		delete(ts.clients, client)
		client.Close()
		fmt.Printf("  Client disconnected: %s\n", client.RemoteAddr())
	}
}

// getLocalIPs retrieves all local IP addresses
func (ts *TCPServer) getLocalIPs() ([]string, error) {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			ips = append(ips, ipNet.IP.String())
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no local IP addresses found")
	}

	return ips, nil
}

// displayServerInfo shows server connection information
func (ts *TCPServer) displayServerInfo() {
	fmt.Printf("\nTCP Server:\n")
	if ips, err := ts.getLocalIPs(); err == nil {
		fmt.Printf("Addresses:\n")
		for _, ip := range ips {
			fmt.Printf("    tcp://%s:%s\n", ip, ts.config.Server.Port)
		}
	} else {
		fmt.Printf("  Server Address: 0.0.0.0:%s\n", ts.config.Server.Port)
	}
	fmt.Println()
}
