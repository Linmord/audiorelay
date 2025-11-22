package audiorelay

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

// HTTPServer handles HTTP audio stream connections
type HTTPServer struct {
	config *Config
	server *http.Server
	webFS  fs.FS

	// Audio components
	audioCapture *AudioCapture // 添加 AudioCapture 引用

	// Audio stream clients
	streamClients   map[http.ResponseWriter]bool
	streamClientsMu sync.RWMutex

	// Audio data buffer for new clients
	audioBuffer   [][]byte
	audioBufferMu sync.RWMutex
	bufferSize    int

	// Control
	isRunning bool
}

// NewHTTPServer creates a new HTTP server instance
func NewHTTPServer(config *Config, webFS fs.FS, audioCapture *AudioCapture) *HTTPServer {
	return &HTTPServer{
		config:        config,
		webFS:         webFS,
		audioCapture:  audioCapture, // 保存 AudioCapture 引用
		streamClients: make(map[http.ResponseWriter]bool),
		audioBuffer:   make([][]byte, 0),
		bufferSize:    50,
	}
}

// Start begins the HTTP server
func (hs *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// Set up routes
	mux.HandleFunc("/", hs.handleRoot)
	mux.HandleFunc("/stream.wav", hs.handleWavStream) // WAV format stream
	mux.HandleFunc("/status", hs.handleStatus)
	mux.HandleFunc("/debug", hs.handleDebug)

	hs.server = &http.Server{
		Addr:         ":" + hs.config.Server.HttpPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // No timeout for streaming connections
	}

	hs.isRunning = true

	// Display server information
	hs.displayServerInfo()

	// Start HTTP server
	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("  HTTP server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTP server
func (hs *HTTPServer) Stop() {
	hs.isRunning = false

	if hs.server != nil {
		hs.server.Close()
	}

	// Close all stream connections
	hs.streamClientsMu.Lock()
	for client := range hs.streamClients {
		if flusher, ok := client.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	hs.streamClients = make(map[http.ResponseWriter]bool)
	hs.streamClientsMu.Unlock()

	fmt.Println(" HTTP server stopped")
}

// Broadcast sends audio data to all connected clients
func (hs *HTTPServer) Broadcast(data []byte) {
	// Broadcast to HTTP stream clients
	hs.broadcastHTTPStream(data)

	// Buffer audio data for new clients
	hs.bufferAudioData(data)
}

// bufferAudioData keeps recent audio data for new clients
func (hs *HTTPServer) bufferAudioData(data []byte) {
	hs.audioBufferMu.Lock()
	defer hs.audioBufferMu.Unlock()

	hs.audioBuffer = append(hs.audioBuffer, data)

	// Keep only the last bufferSize frames
	if len(hs.audioBuffer) > hs.bufferSize {
		hs.audioBuffer = hs.audioBuffer[len(hs.audioBuffer)-hs.bufferSize:]
	}
}

// broadcastHTTPStream sends data to HTTP stream clients
func (hs *HTTPServer) broadcastHTTPStream(data []byte) {
	hs.streamClientsMu.RLock()
	defer hs.streamClientsMu.RUnlock()

	if len(hs.streamClients) == 0 {
		return
	}

	failedClients := make([]http.ResponseWriter, 0)

	for client := range hs.streamClients {
		_, err := client.Write(data)
		if err != nil {
			failedClients = append(failedClients, client)
		} else {
			// Flush the data to client
			if flusher, ok := client.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}

	// Clean up failed clients
	if len(failedClients) > 0 {
		go hs.cleanupStreamClients(failedClients)
	}
}

// GetClientCount returns the number of connected clients
func (hs *HTTPServer) GetClientCount() int {
	hs.streamClientsMu.RLock()
	defer hs.streamClientsMu.RUnlock()
	return len(hs.streamClients)
}

// handleRoot serves the web interface
func (hs *HTTPServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Serve the embedded HTML file
	htmlContent, err := webFS.ReadFile("web/index.html")
	if err != nil {
		// Fallback: serve a simple HTML page if embedded file is not found
		http.Error(w, "Web interface not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}

// handleWavStream handles WAV format audio streaming
func (hs *HTTPServer) handleWavStream(w http.ResponseWriter, r *http.Request) {
	log.Printf("🎵 WAV audio stream connected: %s", r.RemoteAddr)

	// Set headers for WAV stream
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Write WAV header
	hs.writeWAVHeader(w)

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Send buffered audio data to new client
	hs.sendBufferedAudio(w)

	// Add client to stream clients
	hs.addStreamClient(w)

	// Keep connection alive
	<-r.Context().Done()

	// Remove client when connection closes
	hs.removeStreamClient(w)
	log.Printf("🎵 WAV audio stream disconnected: %s", r.RemoteAddr)
}

// writeWAVHeader writes WAV file header
func (hs *HTTPServer) writeWAVHeader(w http.ResponseWriter) {
	sampleRate := int(hs.config.Audio.SampleRate)
	channels := hs.config.Audio.Channels
	bitsPerSample := 16
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	// RIFF header
	w.Write([]byte("RIFF"))
	w.Write([]byte{0xff, 0xff, 0xff, 0xff}) // File size (unknown for stream)
	w.Write([]byte("WAVE"))

	// Format chunk
	w.Write([]byte("fmt "))
	w.Write([]byte{16, 0, 0, 0})                                                                                                               // Chunk size
	w.Write([]byte{1, 0})                                                                                                                      // Audio format (PCM)
	w.Write([]byte{byte(channels), 0})                                                                                                         // Number of channels
	w.Write([]byte{byte(sampleRate & 0xff), byte((sampleRate >> 8) & 0xff), byte((sampleRate >> 16) & 0xff), byte((sampleRate >> 24) & 0xff)}) // Sample rate
	w.Write([]byte{byte(byteRate & 0xff), byte((byteRate >> 8) & 0xff), byte((byteRate >> 16) & 0xff), byte((byteRate >> 24) & 0xff)})         // Byte rate
	w.Write([]byte{byte(blockAlign), 0})                                                                                                       // Block align
	w.Write([]byte{byte(bitsPerSample), 0})                                                                                                    // Bits per sample

	// Data chunk
	w.Write([]byte("data"))
	w.Write([]byte{0xff, 0xff, 0xff, 0xff}) // Data size (unknown for stream)
}

// sendBufferedAudio sends recent audio data to a new client
func (hs *HTTPServer) sendBufferedAudio(w http.ResponseWriter) {
	hs.audioBufferMu.RLock()
	defer hs.audioBufferMu.RUnlock()

	for _, data := range hs.audioBuffer {
		w.Write(data)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// handleStatus returns server status information
func (hs *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	clientCount := hs.GetClientCount()

	actualBufferSize := 0
	if hs.audioCapture != nil {
		actualBufferSize = hs.audioCapture.GetActualBufferSize()
	}

	status := map[string]interface{}{
		"status":             "running",
		"clients":            clientCount,
		"sample_rate":        hs.config.Audio.SampleRate,
		"channels":           hs.config.Audio.Channels,
		"buffer_size":        hs.config.Audio.BufferSize,
		"actual_buffer_size": actualBufferSize,
		"processing": map[string]interface{}{
			"silence_detection": hs.config.Processing.SilenceDetection,
			"silence_threshold": hs.config.Processing.SilenceThreshold,
			"volume_multiplier": hs.config.Processing.VolumeMultiplier,
		},
		"timestamp":     time.Now().Unix(),
		"server_uptime": time.Since(startTime).Seconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewEncoder(w).Encode(status)
}

// handleDebug returns debug information
func (hs *HTTPServer) handleDebug(w http.ResponseWriter, r *http.Request) {
	clientCount := hs.GetClientCount()
	historyBufferSize := len(hs.audioBuffer)

	// Get actual audio buffer size
	actualAudioBufferSize := 0
	if hs.audioCapture != nil {
		actualAudioBufferSize = hs.audioCapture.GetActualBufferSize()
	}

	debugInfo := map[string]interface{}{
		"clients": clientCount,
		"buffers": map[string]interface{}{
			"audio_history_frames": historyBufferSize,          // Current number of frames in history buffer
			"audio_history_max":    hs.bufferSize,              // Maximum capacity of history buffer
			"config_buffer_size":   hs.config.Audio.BufferSize, // Configured audio buffer size
			"actual_buffer_size":   actualAudioBufferSize,      // Actual audio buffer size in use
		},
		"audio_config": map[string]interface{}{
			"sample_rate": hs.config.Audio.SampleRate,
			"channels":    hs.config.Audio.Channels,
		},
		"processing": map[string]interface{}{
			"silence_detection": hs.config.Processing.SilenceDetection,
			"silence_threshold": hs.config.Processing.SilenceThreshold,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewEncoder(w).Encode(debugInfo)
}

// addStreamClient adds a new HTTP stream client
func (hs *HTTPServer) addStreamClient(w http.ResponseWriter) {
	hs.streamClientsMu.Lock()
	defer hs.streamClientsMu.Unlock()
	hs.streamClients[w] = true
	log.Printf("  Total stream clients: %d", len(hs.streamClients))
}

// removeStreamClient removes an HTTP stream client
func (hs *HTTPServer) removeStreamClient(w http.ResponseWriter) {
	hs.streamClientsMu.Lock()
	defer hs.streamClientsMu.Unlock()
	delete(hs.streamClients, w)
	log.Printf("  Total stream clients: %d", len(hs.streamClients))
}

// cleanupStreamClients removes failed stream clients
func (hs *HTTPServer) cleanupStreamClients(failedClients []http.ResponseWriter) {
	hs.streamClientsMu.Lock()
	defer hs.streamClientsMu.Unlock()
	for _, client := range failedClients {
		delete(hs.streamClients, client)
	}
	log.Printf("  Total stream clients after cleanup: %d", len(hs.streamClients))
}

// displayServerInfo shows HTTP server connection information
func (hs *HTTPServer) displayServerInfo() {
	fmt.Printf("HTTP Server:\n")
	if ips, err := hs.getLocalIPs(); err == nil {
		fmt.Printf("  Stream URLs:\n")
		for _, ip := range ips {
			fmt.Printf("    http://%s:%s/stream.wav\n", ip, hs.config.Server.HttpPort)
			fmt.Printf("    http://%s:%s (Web interface)\n", ip, hs.config.Server.HttpPort)
		}
	} else {
		fmt.Printf("  Audio Stream: http://0.0.0.0:%s/stream.wav\n", hs.config.Server.HttpPort)
		fmt.Printf("  Web Interface: http://0.0.0.0:%s\n", hs.config.Server.HttpPort)
	}
	fmt.Println()
}

// getLocalIPs retrieves all local IP addresses
func (hs *HTTPServer) getLocalIPs() ([]string, error) {
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

// Global variable to track server start time
var startTime = time.Now()
