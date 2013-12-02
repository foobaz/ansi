package ansi

import (
	"bytes"
	"io"
	"strconv"
)

type sequence []int

type char struct {
	rendition sequence // select graphic rendition
	code      byte     // in code page 437 encoding
	set       bool     // whether a character exists at this position
}

func decodeSequence(buf []byte) sequence {
	exploded := bytes.Split(buf, []byte{byte(';')})

	s := make(sequence, len(exploded))
	for i, subslice := range exploded {
		var err error
		s[i], err = strconv.Atoi(string(subslice))
		if err != nil {
			return nil
		}
	}
	return s
}

func (s sequence) singleValue(x int) int {
	if len(s) == 1 {
		return s[0]
	} else {
		return x
	}
}

func nextPot(x int) int {
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	return x + 1
}

func writeRendition(w io.Writer, buf []byte, s sequence) (n int, err error) {
	// rendition has changed, write escape sequence
	buf = append(buf, 0x1B)
	buf = append(buf, 0x5B)
	var subsequent bool
	for _, x := range s {
		if subsequent {
			buf = append(buf, byte(';'))
		}
		subsequent = true
		numberString := strconv.Itoa(x)
		buf = append(buf, []byte(numberString)...)
	}
	buf = append(buf, byte('m'))
	return w.Write(buf)
}
