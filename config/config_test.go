package config

import (
	"os"
	"testing"
)

// TestWeakRatioConfig tests parsing of weakRatio configuration parameter
func TestWeakRatioConfig(t *testing.T) {
	// Create temporary config file
	content := `
weakRatio 50
weakWrites 75
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Parse config
	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify weakRatio
	if c.WeakRatio != 50 {
		t.Errorf("WeakRatio = %d, want 50", c.WeakRatio)
	}

	// Verify weakWrites
	if c.WeakWrites != 75 {
		t.Errorf("WeakWrites = %d, want 75", c.WeakWrites)
	}
}

// TestWeakRatioDefault tests default value of weakRatio (should be 0)
func TestWeakRatioDefault(t *testing.T) {
	// Create temporary config file without weakRatio
	content := `
writes 100
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Parse config
	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Default should be 0 (all strong commands)
	if c.WeakRatio != 0 {
		t.Errorf("Default WeakRatio = %d, want 0", c.WeakRatio)
	}

	// Default should be 0
	if c.WeakWrites != 0 {
		t.Errorf("Default WeakWrites = %d, want 0", c.WeakWrites)
	}
}

// TestWeakRatioWithOtherParams tests weakRatio with other config parameters
func TestWeakRatioWithOtherParams(t *testing.T) {
	content := `
protocol curpht
reqs 1000
writes 80
conflicts 10
weakRatio 30
weakWrites 60
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Parse config
	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify all parameters
	if c.Protocol != "curpht" {
		t.Errorf("Protocol = %s, want curpht", c.Protocol)
	}
	if c.Reqs != 1000 {
		t.Errorf("Reqs = %d, want 1000", c.Reqs)
	}
	if c.Writes != 80 {
		t.Errorf("Writes = %d, want 80", c.Writes)
	}
	if c.Conflicts != 10 {
		t.Errorf("Conflicts = %d, want 10", c.Conflicts)
	}
	if c.WeakRatio != 30 {
		t.Errorf("WeakRatio = %d, want 30", c.WeakRatio)
	}
	if c.WeakWrites != 60 {
		t.Errorf("WeakWrites = %d, want 60", c.WeakWrites)
	}
}

// TestWeakRatioEdgeCases tests edge cases for weakRatio
func TestWeakRatioEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		weakRatio int
	}{
		{
			name:      "weakRatio 0",
			content:   "weakRatio 0",
			weakRatio: 0,
		},
		{
			name:      "weakRatio 100",
			content:   "weakRatio 100",
			weakRatio: 100,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "test_config_*.conf")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(f.Name())

			if _, err := f.WriteString(tc.content); err != nil {
				t.Fatal(err)
			}
			f.Close()

			c, err := Read(f.Name(), "test")
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if c.WeakRatio != tc.weakRatio {
				t.Errorf("WeakRatio = %d, want %d", c.WeakRatio, tc.weakRatio)
			}
		})
	}
}

// TestClientThreadsConfig tests parsing of clientThreads configuration parameter
func TestClientThreadsConfig(t *testing.T) {
	content := `
clientThreads 4
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if c.ClientThreads != 4 {
		t.Errorf("ClientThreads = %d, want 4", c.ClientThreads)
	}
}

// TestClientThreadsDefault tests default value of clientThreads (should be 0)
func TestClientThreadsDefault(t *testing.T) {
	content := `
writes 100
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Default should be 0 (use clones behavior)
	if c.ClientThreads != 0 {
		t.Errorf("Default ClientThreads = %d, want 0", c.ClientThreads)
	}
}

// TestClientThreadsWithOtherParams tests clientThreads with other config parameters
func TestClientThreadsWithOtherParams(t *testing.T) {
	content := `
protocol curpht
reqs 1000
writes 80
weakRatio 50
clientThreads 8
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if c.Protocol != "curpht" {
		t.Errorf("Protocol = %s, want curpht", c.Protocol)
	}
	if c.Reqs != 1000 {
		t.Errorf("Reqs = %d, want 1000", c.Reqs)
	}
	if c.WeakRatio != 50 {
		t.Errorf("WeakRatio = %d, want 50", c.WeakRatio)
	}
	if c.ClientThreads != 8 {
		t.Errorf("ClientThreads = %d, want 8", c.ClientThreads)
	}
}

// TestZipfSkewConfig tests parsing of zipfSkew configuration parameter
func TestZipfSkewConfig(t *testing.T) {
	content := `
keySpace 100000
zipfSkew 0.99
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if c.KeySpace != 100000 {
		t.Errorf("KeySpace = %d, want 100000", c.KeySpace)
	}

	if c.ZipfSkew != 0.99 {
		t.Errorf("ZipfSkew = %f, want 0.99", c.ZipfSkew)
	}
}

// TestZipfSkewDefault tests default values of Zipf parameters
func TestZipfSkewDefault(t *testing.T) {
	content := `
writes 100
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Default should be 0 (uniform distribution)
	if c.KeySpace != 0 {
		t.Errorf("Default KeySpace = %d, want 0", c.KeySpace)
	}
	if c.ZipfSkew != 0 {
		t.Errorf("Default ZipfSkew = %f, want 0", c.ZipfSkew)
	}
}

// TestZipfSkewWithOtherParams tests zipfSkew with other config parameters
func TestZipfSkewWithOtherParams(t *testing.T) {
	content := `
protocol curpht
reqs 1000
keySpace 50000
zipfSkew 1.5
weakRatio 50
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if c.Protocol != "curpht" {
		t.Errorf("Protocol = %s, want curpht", c.Protocol)
	}
	if c.KeySpace != 50000 {
		t.Errorf("KeySpace = %d, want 50000", c.KeySpace)
	}
	if c.ZipfSkew != 1.5 {
		t.Errorf("ZipfSkew = %f, want 1.5", c.ZipfSkew)
	}
	if c.WeakRatio != 50 {
		t.Errorf("WeakRatio = %d, want 50", c.WeakRatio)
	}
}

// TestGetNumClientThreads tests the GetNumClientThreads method
func TestGetNumClientThreads(t *testing.T) {
	tests := []struct {
		name          string
		clientThreads int
		clones        int
		expected      int
	}{
		{
			name:          "ClientThreads set, overrides Clones",
			clientThreads: 4,
			clones:        2,
			expected:      4,
		},
		{
			name:          "ClientThreads set to 1",
			clientThreads: 1,
			clones:        5,
			expected:      1,
		},
		{
			name:          "ClientThreads 0, falls back to Clones+1",
			clientThreads: 0,
			clones:        3,
			expected:      4, // Clones + 1
		},
		{
			name:          "Both 0, returns 1",
			clientThreads: 0,
			clones:        0,
			expected:      1, // 0 + 1
		},
		{
			name:          "High ClientThreads count",
			clientThreads: 16,
			clones:        0,
			expected:      16,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{
				ClientThreads: tc.clientThreads,
				Clones:        tc.clones,
			}

			result := c.GetNumClientThreads()
			if result != tc.expected {
				t.Errorf("GetNumClientThreads() = %d, want %d", result, tc.expected)
			}
		})
	}
}

// TestGetClientOffset tests the GetClientOffset method
func TestGetClientOffset(t *testing.T) {
	tests := []struct {
		name          string
		clientThreads int
		clones        int
		clients       []string
		alias         string
		expected      int
	}{
		{
			name:          "First client with 4 threads",
			clientThreads: 4,
			clones:        0,
			clients:       []string{"client0", "client1", "client2"},
			alias:         "client0",
			expected:      0, // 4 * 0
		},
		{
			name:          "Second client with 4 threads",
			clientThreads: 4,
			clones:        0,
			clients:       []string{"client0", "client1", "client2"},
			alias:         "client1",
			expected:      4, // 4 * 1
		},
		{
			name:          "Third client with 4 threads",
			clientThreads: 4,
			clones:        0,
			clients:       []string{"client0", "client1", "client2"},
			alias:         "client2",
			expected:      8, // 4 * 2
		},
		{
			name:          "Client with 8 threads",
			clientThreads: 8,
			clones:        0,
			clients:       []string{"a", "b", "c"},
			alias:         "c",
			expected:      16, // 8 * 2
		},
		{
			name:          "Fallback to Clones+1",
			clientThreads: 0,
			clones:        3,
			clients:       []string{"x", "y"},
			alias:         "y",
			expected:      4, // (3+1) * 1
		},
		{
			name:          "Client not in list",
			clientThreads: 4,
			clones:        0,
			clients:       []string{"a", "b"},
			alias:         "notfound",
			expected:      0, // Not found, returns 0
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{
				ClientThreads: tc.clientThreads,
				Clones:        tc.clones,
			}

			result := c.GetClientOffset(tc.clients, tc.alias)
			if result != tc.expected {
				t.Errorf("GetClientOffset() = %d, want %d", result, tc.expected)
			}
		})
	}
}

// TestGetClientOffsetWithParsedConfig tests GetClientOffset with a parsed config
func TestGetClientOffsetWithParsedConfig(t *testing.T) {
	content := `
clientThreads 4
`
	f, err := os.CreateTemp("", "test_config_*.conf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, err := Read(f.Name(), "test")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify GetNumClientThreads
	if c.GetNumClientThreads() != 4 {
		t.Errorf("GetNumClientThreads() = %d, want 4", c.GetNumClientThreads())
	}

	// Verify GetClientOffset
	clients := []string{"client0", "client1", "client2"}
	offset := c.GetClientOffset(clients, "client1")
	if offset != 4 {
		t.Errorf("GetClientOffset() = %d, want 4", offset)
	}
}

// TestMultiThreadedClientIDUniqueness verifies that client IDs are unique across threads
func TestMultiThreadedClientIDUniqueness(t *testing.T) {
	c := &Config{
		ClientThreads: 4,
	}

	clients := []string{"client0", "client1", "client2"}

	// Calculate all client IDs that would be assigned
	ids := make(map[int]bool)
	numThreads := c.GetNumClientThreads()

	for _, client := range clients {
		baseOffset := c.GetClientOffset(clients, client)
		for threadIdx := 0; threadIdx < numThreads; threadIdx++ {
			clientID := baseOffset + threadIdx
			if ids[clientID] {
				t.Errorf("Duplicate client ID: %d for client %s thread %d", clientID, client, threadIdx)
			}
			ids[clientID] = true
		}
	}

	// Verify we have exactly numClients * numThreads unique IDs
	expectedIDs := len(clients) * numThreads
	if len(ids) != expectedIDs {
		t.Errorf("Expected %d unique IDs, got %d", expectedIDs, len(ids))
	}
}
