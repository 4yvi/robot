package audio

import (
	"context"
	"time"
)

func after(ms int) <-chan time.Time { return time.After(time.Duration(ms) * time.Millisecond) }

// Record captures d of audio from r and returns the samples.
func Record(ctx context.Context, r Recorder, d time.Duration) ([]int16, error) {
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	ch, err := r.Start(cctx)
	if err != nil {
		return nil, err
	}
	var all []int16
	for c := range ch {
		all = append(all, c...)
	}
	return all, nil
}
