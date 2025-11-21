package audiorelay

import (
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/gordonklaus/portaudio"
)

// AudioRelay is the main audio relay service
type AudioRelay struct {
	config *Config
	webFS  fs.FS // 添加 webFS 字段

	// Components
	audioCapture *AudioCapture
	deviceMgr    *DeviceManager
	tcpServer    *TCPServer
	httpServer   *HTTPServer

	// Control
	isRunning bool
}

// New creates a new AudioRelay instance with the given configuration
func New(config *Config, webFS fs.FS) *AudioRelay {
	return &AudioRelay{
		config:       config,
		webFS:        webFS, // 初始化 webFS
		deviceMgr:    NewDeviceManager(),
		audioCapture: NewAudioCapture(config),
	}
}

// Start begins the audio relay service
func (ar *AudioRelay) Start() error {
	if ar.isRunning {
		return fmt.Errorf("service is already running")
	}

	fmt.Println("🎧 Audio Relay Service Starting...")
	fmt.Println("==================================")

	// Initialize device manager
	if err := ar.deviceMgr.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize device manager: %v", err)
	}

	// Select audio input device
	selectedDevice, err := ar.selectAudioDevice()
	if err != nil {
		return fmt.Errorf("failed to select audio device: %v", err)
	}

	// Initialize audio capture
	if err := ar.audioCapture.Initialize(selectedDevice); err != nil {
		return fmt.Errorf("failed to initialize audio capture: %v", err)
	}

	// Start protocol servers
	if err := ar.startProtocolServers(); err != nil {
		return fmt.Errorf("failed to start protocol servers: %v", err)
	}

	// Set up audio data callback to broadcast to all clients
	ar.audioCapture.SetDataCallback(ar.broadcastAudioData)

	// Start audio capture
	if err := ar.audioCapture.Start(); err != nil {
		return fmt.Errorf("failed to start audio capture: %v", err)
	}

	ar.isRunning = true

	fmt.Println(" Audio Relay Service Started Successfully")
	fmt.Printf("🎵 Sample Rate: %.0f Hz, Channels: %d\n",
		ar.config.Audio.SampleRate, ar.config.Audio.Channels)
	fmt.Println("==================================")
	fmt.Println("")

	return nil
}

// Stop gracefully shuts down the audio relay service
func (ar *AudioRelay) Stop() {
	if !ar.isRunning {
		return
	}

	fmt.Println("\n×Shutting down Audio Relay Service...")

	// Stop audio capture
	if ar.audioCapture != nil {
		ar.audioCapture.Stop()
	}

	// Stop protocol servers
	ar.stopProtocolServers()

	ar.isRunning = false
	fmt.Println(" Audio Relay Service Stopped")
}

// selectAudioDevice handles audio device selection based on configuration
func (ar *AudioRelay) selectAudioDevice() (*portaudio.DeviceInfo, error) {
	// Use specified device if configured
	if ar.config.Audio.DeviceName != "" {
		device, err := ar.deviceMgr.GetDeviceByName(ar.config.Audio.DeviceName)
		if err != nil {
			return nil, fmt.Errorf("specified device not found: %v", err)
		}
		return device, nil
	}

	// Auto-select BlackHole device if preferred
	if ar.config.Audio.PreferBlackHole {
		if device := ar.deviceMgr.AutoDetectBlackHole(); device != nil {
			fmt.Printf(" Auto-selected BlackHole device: %s\n", device.Name)
			return device, nil
		}
	}

	// Auto-select default device if configured
	if ar.config.Audio.AutoSelect {
		device, err := ar.deviceMgr.GetDefaultInputDevice()
		if err != nil {
			return nil, fmt.Errorf("failed to get default device: %v", err)
		}
		fmt.Printf(" Auto-selected default device: %s\n", device.Name)
		return device, nil
	}

	// Interactive device selection
	fmt.Println("\n🎧 Available Audio Input Devices:")
	return ar.deviceMgr.SelectInputDevice()
}

// startProtocolServers starts all enabled protocol servers
func (ar *AudioRelay) startProtocolServers() error {
	// Start TCP server if enabled
	if ar.config.Protocols.TCP.Enabled {
		ar.tcpServer = NewTCPServer(ar.config)
		if err := ar.tcpServer.Start(); err != nil {
			return fmt.Errorf("failed to start TCP server: %v", err)
		}
	}

	// Start HTTP server if enabled
	if ar.config.Protocols.HTTP.Enabled {
		ar.httpServer = NewHTTPServer(ar.config, ar.webFS, ar.audioCapture)
		if err := ar.httpServer.Start(); err != nil {
			return fmt.Errorf("failed to start HTTP server: %v", err)
		}
	}

	return nil
}

// stopProtocolServers stops all running protocol servers
func (ar *AudioRelay) stopProtocolServers() {
	if ar.tcpServer != nil {
		ar.tcpServer.Stop()
	}
	if ar.httpServer != nil {
		ar.httpServer.Stop()
	}
}

// broadcastAudioData broadcasts audio data to all connected clients
func (ar *AudioRelay) broadcastAudioData(audioData []byte) {
	// Broadcast to TCP clients
	if ar.tcpServer != nil && ar.config.Protocols.TCP.Enabled {
		ar.tcpServer.Broadcast(audioData)
	}

	// Broadcast to HTTP stream clients
	if ar.httpServer != nil && ar.config.Protocols.HTTP.Enabled {
		ar.httpServer.Broadcast(audioData)
	}
}

type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// StartWithConfig starts the audio relay service with configuration file
func StartWithConfig(configPath string) error {
	// Load configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		return fmt.Errorf("PortAudio initialization failed: %v", err)
	}
	defer portaudio.Terminate()

	var webFS fs.FS = emptyFS{}

	// Create and start relay
	relay := New(config, webFS)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start service
	fmt.Println("👊Starting Audio Relay Service...")
	if err := relay.Start(); err != nil {
		return err
	}

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\n×Shutting down audio relay...")
	relay.Stop()

	fmt.Println("√ Service stopped successfully")
	return nil
}
