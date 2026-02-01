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
