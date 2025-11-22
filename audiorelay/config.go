package audiorelay

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config defines the configuration structure for audio relay
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Audio      AudioConfig      `mapstructure:"audio"`
	Processing ProcessingConfig `mapstructure:"processing"`
	Protocols  ProtocolsConfig  `mapstructure:"protocols"`
}

type ServerConfig struct {
	Port     string `mapstructure:"port"`      // TCP server port
	HttpPort string `mapstructure:"http_port"` // HTTP server port
}

type AudioConfig struct {
	SampleRate      float64 `mapstructure:"sample_rate"`      // Audio sample rate in Hz
	Channels        int     `mapstructure:"channels"`         // Number of audio channels
	BufferSize      int     `mapstructure:"buffer_size"`      // Audio buffer size in samples
	DeviceName      string  `mapstructure:"device_name"`      // Specific audio device name
	AutoSelect      bool    `mapstructure:"auto_select"`      // Auto select default device
	PreferBlackHole bool    `mapstructure:"prefer_blackhole"` // Prefer BlackHole virtual devices
}

type ProcessingConfig struct {
	SilenceDetection bool    `mapstructure:"silence_detection"` // Enable/disable silence detection
	SilenceThreshold int     `mapstructure:"silence_threshold"` // Silence detection threshold
	VolumeMultiplier float64 `mapstructure:"volume_multiplier"` // Volume adjustment
	ClipThreshold    int16   `mapstructure:"clip_threshold"`    // Audio clipping threshold
}

type ProtocolsConfig struct {
	TCP  ProtocolConfig `mapstructure:"tcp"`  // TCP protocol configuration
	HTTP HTTPConfig     `mapstructure:"http"` // HTTP protocol configuration
}

type ProtocolConfig struct {
	Enabled bool `mapstructure:"enabled"` // Enable the protocol
}

type HTTPConfig struct {
	Enabled bool `mapstructure:"enabled"` // Enable HTTP server
	// StreamPath string `mapstructure:"stream_path"` // WebSocket stream path
}

// LoadConfig loads configuration using Viper
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Configuration setup
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read configuration
	if err := v.ReadInConfig(); err != nil {
		log.Printf("Warning: Could not read config file: %v", err)
		log.Println("Using default configuration")
	}

	// Unmarshal configuration
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	log.Printf("Configuration loaded: %s", v.ConfigFileUsed())
	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", "12345")
	v.SetDefault("server.http_port", "8080")

	// Audio defaults
	v.SetDefault("audio.sample_rate", 48000)
	v.SetDefault("audio.channels", 2)
	v.SetDefault("audio.buffer_size", 0)
	v.SetDefault("audio.device_name", "")
	v.SetDefault("audio.auto_select", false)
	v.SetDefault("audio.prefer_blackhole", true)

	// Processing defaults
	v.SetDefault("processing.silence_detection", true) // Enable silence detection by default
	v.SetDefault("processing.silence_threshold", 1000)
	v.SetDefault("processing.volume_multiplier", 1.0)
	v.SetDefault("processing.clip_threshold", 28000)

	// Protocols defaults
	v.SetDefault("protocols.tcp.enabled", true)
	v.SetDefault("protocols.http.enabled", true)
}

// Validate checks if configuration parameters are valid
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}
	if c.Server.HttpPort == "" {
		return fmt.Errorf("HTTP server port cannot be empty")
	}
	if c.Audio.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}
	if c.Audio.Channels <= 0 {
		return fmt.Errorf("channels must be positive")
	}
	if c.Audio.BufferSize < 0 {
		return fmt.Errorf("buffer size must be positive")
	}
	// if c.Protocols.HTTP.StreamPath == "" {
	// 	return fmt.Errorf("HTTP stream path cannot be empty")
	// }
	return nil
}

// CreateDefaultConfig creates a default configuration file
func CreateDefaultConfig(filename string) error {
	v := viper.New()
	setDefaults(v)
	return v.WriteConfigAs(filename)
}
