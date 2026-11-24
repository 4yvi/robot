package tts

import (
	"strings"
	"unicode/utf8"
)

const (
	minSentence = 12  // shorter pieces are merged with the next one
	maxSentence = 220 // longer pieces are cut at a comma or space
)

func isTerminator(r rune) bool { return r == '.' || r == '!' || r == '?' || r == '…' || r == '\n' }

// SentenceBuffer accumulates streamed text and releases finished sentences.
type SentenceBuffer struct{ buf strings.Builder }

// Push adds text and returns any sentences that are now complete.
func (b *SentenceBuffer) Push(s string) []string {
	b.buf.WriteString(s)
	return b.drain(false)
}

// Flush returns whatever is left as a final sentence.
func (b *SentenceBuffer) Flush() []string { return b.drain(true) }

func (b *SentenceBuffer) drain(final bool) []string {
	text := b.buf.String()
	var out []string
	start := 0
	runes := []rune(text)
	idx := 0
	pos := 0 // byte offset of runes[idx]
	for idx < len(runes) {
		r := runes[idx]
		size := utf8.RuneLen(r)
		if isTerminator(r) {
			// include a run of terminators/closing quotes
			end := idx + 1
			endPos := pos + size
			for end < len(runes) && (isTerminator(runes[end]) || runes[end] == '"' || runes[end] == '»' || runes[end] == ')') {
				endPos += utf8.RuneLen(runes[end])
				end++
			}
			// A terminator at the very end may still be followed by more text
			// (e.g. "3." then "5"), so only cut when something follows or we flush.
			complete := end < len(runes) && (runes[end] == ' ' || runes[end] == '\n') || final && end == len(runes) || r == '\n'
			if complete {
				piece := strings.TrimSpace(text[start:endPos])
				if utf8.RuneCountInString(piece) >= minSentence || final && end == len(runes) {
					out = append(out, piece)
					start = endPos
				}
			}
			idx, pos = end, endPos
			continue
		}
		idx++
		pos += size
	}
	rest := text[start:]
	if final {
		if t := strings.TrimSpace(rest); t != "" {
			out = append(out, t)
		}
		rest = ""
	}
	b.buf.Reset()
	b.buf.WriteString(rest)
	var res []string
	for _, p := range out {
		res = append(res, cutLong(p)...)
	}
	return res
}

// cutLong splits an overlong sentence at commas/spaces so synthesis stays quick.
func cutLong(s string) []string {
	var out []string
	for utf8.RuneCountInString(s) > maxSentence {
		r := []rune(s)
		cut := -1
		for i := maxSentence; i > maxSentence/2; i-- {
			if r[i] == ',' || r[i] == ';' || r[i] == ':' {
				cut = i + 1
				break
			}
		}
		if cut < 0 {
			for i := maxSentence; i > maxSentence/2; i-- {
				if r[i] == ' ' {
					cut = i
					break
				}
			}
		}
		if cut < 0 {
			cut = maxSentence
		}
		out = append(out, strings.TrimSpace(string(r[:cut])))
		s = strings.TrimSpace(string(r[cut:]))
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// Split breaks a complete text into sentences.
func Split(text string) []string {
	var b SentenceBuffer
	out := b.Push(text)
	return append(out, b.Flush()...)
}
