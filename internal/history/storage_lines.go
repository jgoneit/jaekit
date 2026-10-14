package history

import "io"

// readEventLines enforces the stored-line bound while reading. It leaves the
// complete bytes and all other validation to Parse, without limiting the total
// size of a history containing multiple valid lines.
func readEventLines(source io.Reader) ([]byte, error) {
	return io.ReadAll(&eventLineReader{source: source, line: 1})
}

type eventLineReader struct {
	source io.Reader
	line   int
	size   int
}

func (r *eventLineReader) Read(p []byte) (int, error) {
	// One extra byte distinguishes an allowed newline from an oversized line.
	// Do not read the rest of a damaged line just to locate its delimiter.
	if remaining := MaxLine + 1 - r.size; len(p) > remaining {
		p = p[:remaining]
	}
	n, err := r.source.Read(p)
	if err != nil && err != io.EOF {
		return n, err
	}
	for _, b := range p[:n] {
		if b == '\n' {
			r.line++
			r.size = 0
		} else {
			r.size++
			if r.size > MaxLine {
				return n, fail("history_corrupt", "line %d: the line exceeds %d bytes; the stored history is not repaired or truncated", r.line, MaxLine)
			}
		}
	}
	return n, err
}
