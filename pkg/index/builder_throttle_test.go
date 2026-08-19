package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuilder_SetThrottleDelayLiftsThrottle verifies that a throttle can be
// lifted mid-build (the mechanism behind "search request speeds up the boot
// scan").
func TestBuilder_SetThrottleDelayLiftsThrottle(t *testing.T) {
	dir := t.TempDir()
	const files = 300
	for i := 0; i < files; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%03d.txt", i)), []byte("x"), 0644))
	}

	b := NewBuilder(BuilderOptions{WorkerCount: 1})
	ctx := context.Background()

	// Lift the throttle shortly after the build starts: 300 entries at 5ms
	// each would take ~1.5s fully throttled; boosted it should finish fast.
	go func() {
		time.Sleep(100 * time.Millisecond)
		b.SetThrottleDelay(0)
	}()

	start := time.Now()
	require.NoError(t, b.BuildThrottled(ctx, []string{dir}, 5*time.Millisecond))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 1*time.Second,
		"boosted build should finish well under the fully-throttled ~1.5s, took %v", elapsed)
	assert.Equal(t, files+1, b.Index().Len(), "all files + root dir should be indexed")
}

// TestBuilder_SetThrottleDelayBeforeBuild verifies the delay applies to a
// future build as well.
func TestBuilder_SetThrottleDelayBeforeBuild(t *testing.T) {
	b := NewBuilder(BuilderOptions{})
	b.SetThrottleDelay(0) // must not panic and must disable throttling
	require.NoError(t, b.Build(context.Background(), []string{t.TempDir()}))
}
