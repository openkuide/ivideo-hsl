package pipeline

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
)

// TestRunner_SemaphoresSizedFromConfig pins the contract that NewRunner
// honors cfg.MaxParallel for cpuSem and cfg.MaxParallel*2 for netSem.
func TestRunner_SemaphoresSizedFromConfig(t *testing.T) {
	cases := []struct {
		name           string
		parallelMode   bool
		maxParallel    int
		wantCPUCap     int
		wantNetCap     int
		wantSerialMode bool
	}{
		{"serial: flag off", false, 1, 1, 1, true},
		{"serial: parallel on but maxParallel=1", true, 1, 1, 1, true},
		{"parallel 2", true, 2, 2, 4, false},
		{"parallel 8", true, 8, 8, 16, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{ParallelMode: c.parallelMode, MaxParallel: c.maxParallel}
			r := NewRunner(cfg, nil)
			u := r.Usage()
			if u.CPUCapacity != c.wantCPUCap {
				t.Errorf("CPUCapacity = %d, want %d", u.CPUCapacity, c.wantCPUCap)
			}
			if u.NetCapacity != c.wantNetCap {
				t.Errorf("NetCapacity = %d, want %d", u.NetCapacity, c.wantNetCap)
			}
			isSerial := r.cpuSem == nil
			if isSerial != c.wantSerialMode {
				t.Errorf("serial mode = %v (cpuSem nil: %v), want %v",
					isSerial, r.cpuSem == nil, c.wantSerialMode)
			}
		})
	}
}

// TestGateTracked_EnforcesCapacityAndCountsAccurately proves that the
// semaphore actually limits concurrency at runtime and that the atomic
// counter tracks in-use slots correctly — the two things the TUI gauge
// depends on being true.
func TestGateTracked_EnforcesCapacityAndCountsAccurately(t *testing.T) {
	const capacity = 3
	const attempts = 10
	sem := semaphore.NewWeighted(capacity)
	var counter atomic.Int32

	var peakInUse atomic.Int32
	release := make(chan struct{})

	start := make(chan struct{})
	done := make(chan struct{}, attempts)
	for range attempts {
		go func() {
			<-start
			_ = gateTracked(context.Background(), sem, &counter, func() error {
				// record peak occupancy while holding the slot
				for {
					cur := counter.Load()
					peak := peakInUse.Load()
					if cur <= peak {
						break
					}
					if peakInUse.CompareAndSwap(peak, cur) {
						break
					}
				}
				<-release
				return nil
			})
			done <- struct{}{}
		}()
	}
	close(start)
	// give goroutines time to all acquire what they can, then settle.
	time.Sleep(50 * time.Millisecond)

	if got := counter.Load(); got != int32(capacity) {
		t.Errorf("counter while all slots held = %d, want %d", got, capacity)
	}

	close(release)
	for range attempts {
		<-done
	}

	if got := counter.Load(); got != 0 {
		t.Errorf("counter after all released = %d, want 0", got)
	}
	if got := peakInUse.Load(); got != int32(capacity) {
		t.Errorf("peak in use = %d, want exactly %d (semaphore not enforcing)",
			got, capacity)
	}
}

// TestGateTracked_NilSemaphoreStillCounts ensures the serial path (nil
// semaphore, used when ParallelMode is off) still increments the counter,
// so the TUI gauge shows "CPU 1/1" during serial runs instead of "0/1".
func TestGateTracked_NilSemaphoreStillCounts(t *testing.T) {
	var counter atomic.Int32
	insideCount := int32(-1)
	err := gateTracked(context.Background(), nil, &counter, func() error {
		insideCount = counter.Load()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if insideCount != 1 {
		t.Errorf("counter inside fn (nil sem) = %d, want 1", insideCount)
	}
	if counter.Load() != 0 {
		t.Errorf("counter after return = %d, want 0", counter.Load())
	}
}
