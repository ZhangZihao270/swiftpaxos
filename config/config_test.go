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

// TestHighThreadCountStress tests the system with high thread counts (8, 16, 32, 64)
func TestHighThreadCountStress(t *testing.T) {
	threadCounts := []int{8, 16, 32, 64}
	clientCounts := []int{2, 5, 10}

	for _, numThreads := range threadCounts {
		for _, numClients := range clientCounts {
			t.Run(
				// Using manual string concatenation instead of fmt for test name
				"threads_"+string(rune('0'+numThreads/10))+string(rune('0'+numThreads%10))+"_clients_"+string(rune('0'+numClients/10))+string(rune('0'+numClients%10)),
				func(t *testing.T) {
					testHighThreadCount(t, numThreads, numClients)
				})
		}
	}
}

func testHighThreadCount(t *testing.T, numThreads, numClients int) {
	c := &Config{
		ClientThreads: numThreads,
	}

	// Generate client list
	clients := make([]string, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = "client" + string(rune('0'+i/10)) + string(rune('0'+i%10))
	}

	// Verify GetNumClientThreads
	if c.GetNumClientThreads() != numThreads {
		t.Errorf("GetNumClientThreads() = %d, want %d", c.GetNumClientThreads(), numThreads)
	}

	// Verify all client IDs are unique
	ids := make(map[int]bool)
	for _, client := range clients {
		baseOffset := c.GetClientOffset(clients, client)
		for threadIdx := 0; threadIdx < numThreads; threadIdx++ {
			clientID := baseOffset + threadIdx
			if ids[clientID] {
				t.Errorf("Duplicate client ID: %d", clientID)
			}
			ids[clientID] = true
		}
	}

	// Verify total unique IDs
	expectedIDs := numClients * numThreads
	if len(ids) != expectedIDs {
		t.Errorf("Expected %d unique IDs, got %d", expectedIDs, len(ids))
	}

	// Verify ID range is contiguous (0 to expectedIDs-1)
	for i := 0; i < expectedIDs; i++ {
		if !ids[i] {
			t.Errorf("Missing client ID: %d", i)
		}
	}
}

// TestGetNumClientThreadsConcurrent tests concurrent access to GetNumClientThreads
func TestGetNumClientThreadsConcurrent(t *testing.T) {
	c := &Config{
		ClientThreads: 16,
		Clones:        4,
	}

	clients := []string{"client0", "client1", "client2", "client3"}

	// Run many concurrent goroutines accessing the methods
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			// Each goroutine reads multiple times
			for j := 0; j < 1000; j++ {
				numThreads := c.GetNumClientThreads()
				if numThreads != 16 {
					t.Errorf("Goroutine %d: GetNumClientThreads() = %d, want 16", idx, numThreads)
				}

				clientIdx := idx % len(clients)
				offset := c.GetClientOffset(clients, clients[clientIdx])
				expectedOffset := 16 * clientIdx
				if offset != expectedOffset {
					t.Errorf("Goroutine %d: GetClientOffset() = %d, want %d", idx, offset, expectedOffset)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}
