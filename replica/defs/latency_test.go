package defs

import (
	"os"
	"testing"
	"time"
)

func writeConf(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "latency-*.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestUniformSkipsLocal(t *testing.T) {
	conf := writeConf(t, "uniform 50ms\n")
	defer os.Remove(conf)

	addrs := []string{"10.0.0.1:7070", "10.0.0.2:7070", "10.0.0.3:7070"}
	dt := NewLatencyTable(conf, "10.0.0.1:1234", addrs)
	if dt == nil {
		t.Fatal("expected non-nil LatencyTable")
	}

	// Co-located address should return 0
	if d := dt.WaitDuration("10.0.0.1:5555"); d != 0 {
		t.Errorf("WaitDuration(local) = %v, want 0", d)
	}

	// Remote addresses should return 25ms (half of 50ms RTT)
	want := 25 * time.Millisecond
	if d := dt.WaitDuration("10.0.0.2:5555"); d != want {
		t.Errorf("WaitDuration(remote) = %v, want %v", d, want)
	}
	if d := dt.WaitDuration("10.0.0.3:5555"); d != want {
		t.Errorf("WaitDuration(remote2) = %v, want %v", d, want)
	}
}

func TestUniformSkipsLocalByID(t *testing.T) {
	conf := writeConf(t, "uniform 50ms\n")
	defer os.Remove(conf)

	addrs := []string{"10.0.0.1:7070", "10.0.0.2:7070", "10.0.0.3:7070"}
	dt := NewLatencyTable(conf, "10.0.0.1", addrs)
	if dt == nil {
		t.Fatal("expected non-nil LatencyTable")
	}

	// Replica 0 is co-located, should return 0
	if d := dt.WaitDurationID(0); d != 0 {
		t.Errorf("WaitDurationID(local=0) = %v, want 0", d)
	}

	// Replicas 1 and 2 are remote, should return 25ms
	want := 25 * time.Millisecond
	if d := dt.WaitDurationID(1); d != want {
		t.Errorf("WaitDurationID(remote=1) = %v, want %v", d, want)
	}
	if d := dt.WaitDurationID(2); d != want {
		t.Errorf("WaitDurationID(remote=2) = %v, want %v", d, want)
	}
}

func TestPairwiseSkipsLocal(t *testing.T) {
	conf := writeConf(t, "10.0.0.1 10.0.0.2 40ms\n10.0.0.1 10.0.0.3 80ms\n")
	defer os.Remove(conf)

	addrs := []string{"10.0.0.1:7070", "10.0.0.2:7070", "10.0.0.3:7070"}
	dt := NewLatencyTable(conf, "10.0.0.1:1234", addrs)
	if dt == nil {
		t.Fatal("expected non-nil LatencyTable")
	}

	// Co-located should return 0
	if d := dt.WaitDuration("10.0.0.1:5555"); d != 0 {
		t.Errorf("WaitDuration(local) = %v, want 0", d)
	}
	if d := dt.WaitDurationID(0); d != 0 {
		t.Errorf("WaitDurationID(local=0) = %v, want 0", d)
	}

	// Remote should return half RTT
	if d := dt.WaitDuration("10.0.0.2:5555"); d != 20*time.Millisecond {
		t.Errorf("WaitDuration(.2) = %v, want 20ms", d)
	}
	if d := dt.WaitDuration("10.0.0.3:5555"); d != 40*time.Millisecond {
		t.Errorf("WaitDuration(.3) = %v, want 40ms", d)
	}
	if d := dt.WaitDurationID(1); d != 20*time.Millisecond {
		t.Errorf("WaitDurationID(1) = %v, want 20ms", d)
	}
	if d := dt.WaitDurationID(2); d != 40*time.Millisecond {
		t.Errorf("WaitDurationID(2) = %v, want 40ms", d)
	}
}

func TestNilLatencyTable(t *testing.T) {
	var dt *LatencyTable
	if d := dt.WaitDuration("10.0.0.1:5555"); d != 0 {
		t.Errorf("nil.WaitDuration = %v, want 0", d)
	}
	if d := dt.WaitDurationID(0); d != 0 {
		t.Errorf("nil.WaitDurationID = %v, want 0", d)
	}
}

func TestEmptyConf(t *testing.T) {
	dt := NewLatencyTable("", "10.0.0.1", nil)
	if dt != nil {
		t.Errorf("expected nil for empty conf, got %+v", dt)
	}
}
