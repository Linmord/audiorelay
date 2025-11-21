package audiorelay

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gordonklaus/portaudio"
)

// DeviceManager handles audio device operations
type DeviceManager struct {
	devices []*portaudio.DeviceInfo
}

// NewDeviceManager creates a new device manager instance
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{}
}

// Initialize loads available audio devices
func (dm *DeviceManager) Initialize() error {
	allDevices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get audio devices: %v", err)
	}

	// Filter input devices
	var inputDevices []*portaudio.DeviceInfo
	for _, device := range allDevices {
		if device.MaxInputChannels > 0 {
			inputDevices = append(inputDevices, device)
		}
	}

	if len(inputDevices) == 0 {
		return fmt.Errorf("no available input devices found")
	}

	dm.devices = inputDevices
	return nil
}

// GetInputDevices returns all available input devices
func (dm *DeviceManager) GetInputDevices() ([]*portaudio.DeviceInfo, error) {
	if len(dm.devices) == 0 {
		return nil, fmt.Errorf("no input devices available")
	}
	return dm.devices, nil
}

// GetDefaultInputDevice returns the default input device
func (dm *DeviceManager) GetDefaultInputDevice() (*portaudio.DeviceInfo, error) {
	device, err := portaudio.DefaultInputDevice()
	if err != nil {
		return nil, fmt.Errorf("failed to get default input device: %v", err)
	}
	return device, nil
}

// GetDeviceByName finds a device by its name
func (dm *DeviceManager) GetDeviceByName(name string) (*portaudio.DeviceInfo, error) {
	for _, device := range dm.devices {
		if strings.EqualFold(device.Name, name) {
			return device, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", name)
}

// AutoDetectBlackHole automatically detects BlackHole audio devices
func (dm *DeviceManager) AutoDetectBlackHole() *portaudio.DeviceInfo {
	blackHoleNames := []string{
		"BlackHole 2ch",
		"BlackHole 16ch",
		"BlackHole",
	}

	for _, device := range dm.devices {
		for _, name := range blackHoleNames {
			if strings.Contains(strings.ToLower(device.Name), strings.ToLower(name)) {
				return device
			}
		}
	}
	return nil
}

// SelectInputDevice provides interactive device selection
func (dm *DeviceManager) SelectInputDevice() (*portaudio.DeviceInfo, error) {
	devices, err := dm.GetInputDevices()
	if err != nil {
		return nil, err
	}

	// Display available devices
	fmt.Println("\nAvailable Audio Input Devices:")
	fmt.Println("==============================")

	for i, device := range devices {
		defaultMarker := ""
		defaultDevice, err := portaudio.DefaultInputDevice()
		if err == nil && device.Name == defaultDevice.Name {
			defaultMarker = " (default)"
		}

		fmt.Printf("[%d] %s%s\n", i, device.Name, defaultMarker)
		fmt.Printf("    Input Channels: %d, Sample Rate: %.0f Hz, API: %s\n",
			device.MaxInputChannels, device.DefaultSampleRate,
			device.HostApi.Name)
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Select device number (q to quit): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.ToLower(input) == "q" {
			return nil, fmt.Errorf("device selection cancelled by user")
		}

		index, err := strconv.Atoi(input)
		if err != nil || index < 0 || index >= len(devices) {
			fmt.Printf("Invalid choice, please enter 0-%d\n", len(devices)-1)
			continue
		}

		selectedDevice := devices[index]
		dm.displayDeviceInfo(selectedDevice)

		return selectedDevice, nil
	}
}

// displayDeviceInfo shows detailed information about a device
func (dm *DeviceManager) displayDeviceInfo(device *portaudio.DeviceInfo) {
	fmt.Printf("\nDevice Details:\n")
	fmt.Printf("  Name: %s\n", device.Name)
	fmt.Printf("  Input Channels: %d\n", device.MaxInputChannels)
	fmt.Printf("  Output Channels: %d\n", device.MaxOutputChannels)
	fmt.Printf("  Default Sample Rate: %.0f Hz\n", device.DefaultSampleRate)
	fmt.Printf("  Low Latency: %.1f ms\n", device.DefaultLowInputLatency.Seconds()*1000)
	fmt.Printf("  High Latency: %.1f ms\n", device.DefaultHighInputLatency.Seconds()*1000)
	fmt.Printf("  Host API: %s\n", device.HostApi.Name)
	fmt.Println()
}
