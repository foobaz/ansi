package ansi

import (
	"errors"
	"io"
	"strconv"
)

type Terminal struct {
	page                 [][]char
	rendition            sequence
	bracketed            []byte
	x, y, savedX, savedY int
	escaping, bracketing bool
}

func (t *Terminal) Write(p []byte) (n int, err error) {
	for _, c := range p {
		err = t.WriteByte(c)
		if err != nil {
			return
		}
		n++
	}
	return
}

func (t *Terminal) WriteByte(c byte) error {
	if t.escaping {
		if c > 127 {
			hex := strconv.FormatInt(int64(c), 16)
			errorString := "illegal high byte 0x" + hex + "in escape sequence"
			return errors.New(errorString)
			/*
				t.bracketing = false
				t.escaping = false
				t.bracketed = bracketed[:0]
				t.writeCode(c)
			*/
		}

		if t.bracketing {
			if c > 63 {
				t.bracketing = false
				t.escaping = false
				t.escapeSequence(c, t.bracketed)
				t.bracketed = nil
			} else {
				t.bracketed = append(t.bracketed, c)
			}
		} else {
			if c == 0x5B {
				t.bracketing = true
			} else {
				//log.Printf("%c", c)
				t.escaping = false
				t.escapeSequence(c, nil)
			}
		}
	} else {
		switch c {
		case 0x0A:
			t.y++
		case 0x0D:
			t.x = 0
		case 0x1B:
			t.escaping = true
		default:
			t.writeCode(c)
		}
	}
	return nil
}

func (t *Terminal) WriteTo(w io.Writer) (n int64, err error) {
	var rendition sequence
	var buf []byte
	for _, line := range t.page {
		for _, c := range line {
			// check if rendition has changed
			if len(c.rendition) != len(rendition) {
				var dn int
				dn, err = writeRendition(w, buf, c.rendition)
				if err != nil {
					return
				}
				n += int64(dn)
				buf = buf[:0]
				rendition = c.rendition
			} else {
				for i := range rendition {
					if rendition[i] == c.rendition[i] {
						continue
					}

					var dn int
					dn, err = writeRendition(w, buf, c.rendition)
					if err != nil {
						return
					}
					n += int64(dn)
					buf = buf[:0]
					rendition = c.rendition
					break
				}
			}
			var code byte
			if c.set {
				code = c.code
			} else {
				code = 0x20 // space
			}

			var dn int
			if code < 128 {
				buf = append(buf, code)
				dn, err = w.Write(buf)
				buf = buf[:0]
			} else {
				dn, err = w.Write(CP437toUTF8[code-128])
			}
			if err != nil {
				return
			}
			n += int64(dn)
		}

		var dn int
		buf = append(buf, 0x0A) // newline
		dn, err = w.Write(buf)
		if err != nil {
			return
		}
		n += int64(dn)
		buf = buf[:0]
	}
	return
}

func (t *Terminal) writeCode(c byte) {
	if len(t.page) <= t.y {
		height := t.y + 1
		if cap(t.page) <= t.y {
			capacity := nextPot(height)
			//log.Printf("making [%d]page with capacity %d because t.y == %d", height, capacity, t.y)
			newPage := make([][]char, height, capacity)
			copy(newPage, t.page)
			t.page = newPage
		} else {
			t.page = t.page[:height]
		}
	}

	if len(t.page[t.y]) <= t.x {
		width := t.x + 1
		if cap(t.page[t.y]) <= t.x {
			capacity := nextPot(width)
			newLine := make([]char, width, capacity)
			//log.Printf("making [%d]line with capacity %d because t.x == %d", width, capacity, t.x)
			copy(newLine, t.page[t.y])
			t.page[t.y] = newLine
		} else {
			t.page[t.y] = t.page[t.y][:width]
		}
	}

	t.page[t.y][t.x] = char{t.rendition, c, true}
	t.x++
	/*
		for t.x >= 80 {
			t.x -= 80
			t.y++
		}
	*/
}

func (t *Terminal) escapeSequence(c byte, sequenceBytes []byte) {
	s := decodeSequence(sequenceBytes)
	length := len(s)
	switch rune(c) {
	case 'A': // cursor up
		dy := s.singleValue(1)
		//log.Printf("cursor up by %d", dy)
		if t.y > dy {
			t.y--
		} else {
			t.y = 0
		}
	case 'B': // cursor down
		dy := s.singleValue(1)
		//log.Printf("cursor down by %d", dy)
		t.y += dy
	case 'C': // cursor forward
		dx := s.singleValue(1)
		//log.Printf("cursor forward by %d", dx)
		t.x += dx
	case 'D': // cursor back
		dx := s.singleValue(1)
		//log.Printf("cursor back by %d", dx)
		if t.x > dx {
			t.x--
		} else {
			t.x = 0
		}
	case 'E': // cursor next line
		dy := s.singleValue(1)
		//log.Printf("cursor next line by %d", dy)
		t.y += dy
		t.x = 0
	case 'F': // cursor previous line
		dy := s.singleValue(1)
		//log.Printf("cursor previous line by %d", dy)
		if t.y > dy {
			t.y -= dy
		} else {
			t.y = 0
		}
		t.x = 0
	case 'G': // cursor horizontal absolute
		y := s.singleValue(1) - 1
		//log.Printf("cursor horizontal to %d", y)
		if y < 0 {
			y = 0
		}
		t.y = y
	case 'H', 'f': // cursor position
		var x, y int
		if length > 0 {
			y = s[0] - 1
			if y < 0 {
				y = 0
			}
		} else {
			y = 0
		}
		if length > 1 {
			x = s[1] - 1
			if x < 0 {
				x = 0
			}
		} else {
			x = 0
		}
		t.x = x
		t.y = y
	case 'J': // erase display
		scope := s.singleValue(0)
		switch scope {
		case 1: // clear from cursor to beginning of screen
			height := len(t.page)
			if height > t.y {
				width := len(t.page[t.y])
				if width > t.x+1 {
					width = t.x + 1
				}
				for x := 0; x < width; x++ {
					t.page[t.y][x] = char{}
				}
				// TODO: see if i can clear line
				height = t.y
			}
			for y := 0; y < height; y++ {
				t.page[y] = nil
			}
		case 2: // clear screen
			t.page = nil
			// moving cursor emulates DOS' ANSI.SYS
			t.x = 0
			t.y = 0
		default: // clear from cursor to end of screen
			height := len(t.page)
			if height > t.y {
				// clear characters on this line
				width := len(t.page[t.y])
				for x := t.x; x < width; x++ {
					t.page[t.y][x] = char{}
				}
				if t.x < width {
					t.page[t.y] = t.page[t.y][:t.x]
				}
			}
			for y := t.y; y < height; y++ {
				// clear following lines
				t.page[y] = nil
			}
			if t.y < height {
				t.page = t.page[:t.y]
			}
		}
	case 'K': // erase in line
		scope := s.singleValue(0)
		switch scope {
		case 1: // clear from cursor to beginning of line
			if t.y < len(t.page) {
				width := len(t.page[t.y])
				if width > t.x+1 {
					width = t.x + 1
				}
				for x := 0; x < width; x++ {
					t.page[t.y][x] = char{}
				}
			}
		case 2: // clear entire line
			if t.y < len(t.page) {
				t.page[t.y] = nil
			}
		default: // clear from cursor to end of line
			if t.y < len(t.page) {
				width := len(t.page[t.y])
				for x := t.x; x < width; x++ {
					t.page[t.y][x] = char{}
				}
			}
		}
	case 'S': // scroll up
		dy := s.singleValue(1)
		height := len(t.page) - dy
		if height > 0 {
			capacity := nextPot(height)
			newPage := make([][]char, height, capacity)
			if dy < 0 {
				copy(newPage[-dy:], t.page)
			} else {
				copy(newPage, t.page[dy:])
			}
			t.page = newPage
		} else {
			t.page = nil
		}
	case 'T': // scroll down
		dy := s.singleValue(1)
		height := len(t.page) + dy
		if height > 0 {
			capacity := nextPot(height)
			newPage := make([][]char, height, capacity)
			if dy < 0 {
				copy(newPage, t.page[-dy:])
			} else {
				copy(newPage[dy:], t.page)
			}
			t.page = newPage
		} else {
			t.page = nil
		}
	case 's': // save cursor position
		t.savedX = t.x
		t.savedY = t.y
	case 'u':
		t.x = t.savedX
		t.y = t.savedY
	case 'm':
		//log.Printf("set rendition to %v", s)
		t.rendition = s
	default:
		panic("unrecognized escape sequence: " + string([]byte{c}) + " " + string(t.bracketed))
	}
	/*
		for t.x >= 80 {
			t.y++
			t.x -= 80
		}
	*/
}
