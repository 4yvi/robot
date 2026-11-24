package audio

import (
	"context"
	"errors"
)

// ErrNoSpeech is returned when nobody started talking within the wait time.
var ErrNoSpeech = errors.New("no speech")

// RecordPhrase reads chunks until the VAD reports the end of a phrase (trailing
// silence or max length). If no speech starts within waitMS (when > 0) it
// returns ErrNoSpeech. The returned samples include the leading audio.
func RecordPhrase(ctx context.Context, chunks <-chan []int16, vad *VAD, waitMS int) ([]int16, error) {
	var all []int16
	for {
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		case c, ok := <-chunks:
			if !ok {
				return all, errors.New("audio stream closed")
			}
			all = append(all, c...)
			if vad.Feed(c) {
				return all, nil
			}
			if waitMS > 0 && !vad.HeardSpeech() && vad.TotalMS() >= waitMS {
				return nil, ErrNoSpeech
			}
		}
	}
}
