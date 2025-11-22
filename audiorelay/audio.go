package audiorelay

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

// AudioCapture handles audio capture and processing
type AudioCapture struct {
	config *Config
	stream *portaudio.Stream

	// Audio processing
	buffer       []int16
	dataCallback func([]byte)

	// 添加实际使用的缓冲区大小
	actualBufferSize int

	// Statistics
	statsMu      sync.RWMutex
	frameCount   int64
	bytesSent    int64
	silenceCount int64

	// Control
	mu          sync.RWMutex
	isCapturing bool
	isRunning   bool
}

// NewAudioCapture creates a new audio capture instance
func NewAudioCapture(config *Config) *AudioCapture {
	return &AudioCapture{
		config: config,
	}
}

// Initialize sets up the audio capture with the selected device
func (ac *AudioCapture) Initialize(device *portaudio.DeviceInfo) error {
	// Calculate optimal buffer size for smooth streaming
	ac.actualBufferSize = ac.calculateOptimalBufferSize()
	ac.buffer = make([]int16, ac.actualBufferSize)

	fmt.Printf("🎵 Initializing audio capture:\n")
	fmt.Printf("   Device: %s\n", device.Name)
	fmt.Printf("   Sample Rate: %.0f Hz\n", ac.config.Audio.SampleRate)
	fmt.Printf("   Channels: %d\n", ac.config.Audio.Channels)

	if ac.config.Audio.BufferSize > 0 {
		fmt.Printf("   Buffer Size: %d samples (configured, %.1f ms)\n",
			ac.actualBufferSize, float64(ac.actualBufferSize)/ac.config.Audio.SampleRate*1000)
	} else {
		fmt.Printf("   Buffer Size: %d samples (auto-calculated, %.1f ms)\n",
			ac.actualBufferSize, float64(ac.actualBufferSize)/ac.config.Audio.SampleRate*1000)
	}

	// Open audio stream
	stream, err := portaudio.OpenStream(
		portaudio.StreamParameters{
			Input: portaudio.StreamDeviceParameters{
				Device:   device,
				Channels: ac.config.Audio.Channels,
				Latency:  device.DefaultLowInputLatency,
			},
			SampleRate:      ac.config.Audio.SampleRate,
			FramesPerBuffer: len(ac.buffer),
		},
		ac.buffer,
	)
	if err != nil {
		return fmt.Errorf("failed to open audio stream: %v", err)
	}

	ac.stream = stream
	return nil
}

// calculateOptimalBufferSize calculates the optimal buffer size for smooth streaming
func (ac *AudioCapture) calculateOptimalBufferSize() int {
	// 如果配置了 buffer_size 且大于0，使用配置的值
	if ac.config.Audio.BufferSize > 0 {
		return ac.config.Audio.BufferSize * ac.config.Audio.Channels
	}

	// 如果 buffer_size 为0或未设置，自动计算最佳大小
	// Target around 20ms of audio for good balance between latency and stability
	targetLatency := 0.02 // 20ms
	targetSamples := int(ac.config.Audio.SampleRate * targetLatency)

	// Round to nearest power of two for better performance
	powerOfTwo := 1
	for powerOfTwo < targetSamples {
		powerOfTwo <<= 1
	}

	// Ensure minimum and maximum bounds
	if powerOfTwo < 256 {
		powerOfTwo = 256
	}
	if powerOfTwo > 2048 {
		powerOfTwo = 2048
	}

	result := powerOfTwo * ac.config.Audio.Channels
	log.Printf("  Auto-calculated buffer size: %d samples (from %d Hz, %d channels)",
		result, int(ac.config.Audio.SampleRate), ac.config.Audio.Channels)

	return result
}

// GetActualBufferSize returns the actual buffer size being used
func (ac *AudioCapture) GetActualBufferSize() int {
	return ac.actualBufferSize
}

// SetDataCallback sets the callback function for processed audio data
func (ac *AudioCapture) SetDataCallback(callback func([]byte)) {
	ac.dataCallback = callback
}

// Start begins audio capture
func (ac *AudioCapture) Start() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.isCapturing {
		return fmt.Errorf("audio capture is already running")
	}

	if err := ac.stream.Start(); err != nil {
		return fmt.Errorf("failed to start audio stream: %v", err)
	}

	ac.isCapturing = true
	ac.isRunning = true

	// Start audio processing loop
	go ac.processAudio()

	fmt.Println("√ Audio capture started")
	return nil
}

// Stop gracefully stops audio capture
func (ac *AudioCapture) Stop() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if !ac.isCapturing {
		return
	}

	ac.isRunning = false
	ac.isCapturing = false

	if ac.stream != nil {
		ac.stream.Stop()
		ac.stream.Close()
		ac.stream = nil
	}

	fmt.Println("√ Audio capture stopped")
}

// IsCapturing returns the current capture status
func (ac *AudioCapture) IsCapturing() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.isCapturing
}

// GetStats returns audio capture statistics
func (ac *AudioCapture) GetStats() (frames int64, bytes int64, silence int64) {
	ac.statsMu.RLock()
	defer ac.statsMu.RUnlock()
	return ac.frameCount, ac.bytesSent, ac.silenceCount
}

// processAudio handles the main audio processing loop
func (ac *AudioCapture) processAudio() {
	lastStats := time.Now()
	bytesTransferred := 0
	silenceFrames := 0
	consecutiveErrors := 0

	for ac.isRunning {
		if err := ac.stream.Read(); err != nil {
			log.Printf("Audio read error: %v", err)
			consecutiveErrors++
			if consecutiveErrors > 10 {
				log.Printf("Too many consecutive errors, stopping audio capture")
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		consecutiveErrors = 0

		ac.statsMu.Lock()
		ac.frameCount++
		ac.statsMu.Unlock()

		// Silence detection (optional)
		isSilent := false
		if ac.config.Processing.SilenceDetection {
			isSilent = ac.isSilence(ac.buffer)
			if isSilent {
				silenceFrames++
				ac.statsMu.Lock()
				ac.silenceCount++
				ac.statsMu.Unlock()

				// Skip processing during extended silence to save bandwidth
				if silenceFrames > 30 {
					continue
				}
			} else {
				silenceFrames = 0
			}
		}

		// Process audio data with high quality processing
		processedBuffer := ac.processAudioData(ac.buffer)
		audioData := ac.int16ToBytes(processedBuffer)

		ac.statsMu.Lock()
		ac.bytesSent += int64(len(audioData))
		ac.statsMu.Unlock()

		bytesTransferred += len(audioData)

		// Send data via callback (non-blocking)
		if ac.dataCallback != nil {
			ac.dataCallback(audioData)
		}

		// Display statistics periodically
		if time.Since(lastStats) > 5*time.Second {
			rate := float64(bytesTransferred) / time.Since(lastStats).Seconds() / 1024
			totalFrames, totalBytes, totalSilence := ac.GetStats()

			status := "Streaming"
			if ac.config.Processing.SilenceDetection && silenceFrames > 0 {
				status = "Silent"
			}

			// Use actual buffer size for display
			totalMB := float64(totalBytes) / 1024 / 1024
			silencePercent := 0.0
			if totalFrames > 0 && ac.config.Processing.SilenceDetection {
				silencePercent = float64(totalSilence) / float64(totalFrames) * 100
			}

			// Build status message
			statusMsg := fmt.Sprintf("Audio Status: %s | Frames: %d | Buffer: %d | Total: %.1f MB | Rate: %.1f KB/s",
				status, totalFrames, ac.actualBufferSize, totalMB, rate)

			// Add silence percentage only if silence detection is enabled
			if ac.config.Processing.SilenceDetection {
				statusMsg += fmt.Sprintf(" | Silence: %.1f%%", silencePercent)
			}

			fmt.Println(statusMsg)

			bytesTransferred = 0
			lastStats = time.Now()
		}
	}
}

// isSilence checks if the audio buffer contains silence with improved detection
func (ac *AudioCapture) isSilence(buffer []int16) bool {
	// Use configured silence threshold
	threshold := int16(ac.config.Processing.SilenceThreshold)

	for i := 0; i < len(buffer); i++ {
		if buffer[i] > threshold || buffer[i] < -threshold {
			return false
		}
	}
	return true
}

// processAudioData applies high-quality audio processing
func (ac *AudioCapture) processAudioData(buffer []int16) []int16 {
	processed := make([]int16, len(buffer))

	// Use high-quality processing with minimal distortion
	for i := range buffer {
		// Apply volume adjustment with smooth curve
		sample := float64(buffer[i])

		// Gentle volume adjustment to preserve dynamics
		sample = sample * ac.config.Processing.VolumeMultiplier

		// Soft clipping to prevent harsh distortion
		if sample > float64(ac.config.Processing.ClipThreshold) {
			// Soft clip: gradual roll-off instead of hard limit
			excess := sample - float64(ac.config.Processing.ClipThreshold)
			sample = float64(ac.config.Processing.ClipThreshold) + excess*0.3
		} else if sample < -float64(ac.config.Processing.ClipThreshold) {
			excess := sample + float64(ac.config.Processing.ClipThreshold)
			sample = -float64(ac.config.Processing.ClipThreshold) + excess*0.3
		}

		processed[i] = int16(sample)
	}

	return processed
}

// int16ToBytes converts int16 audio samples to byte array (little-endian)
func (ac *AudioCapture) int16ToBytes(buffer []int16) []byte {
	bytes := make([]byte, len(buffer)*2)
	for i, sample := range buffer {
		// Little-endian format (standard for WAV, Web Audio API, etc.)
		bytes[i*2] = byte(sample & 0xFF)
		bytes[i*2+1] = byte((sample >> 8) & 0xFF)
	}
	return bytes
}
