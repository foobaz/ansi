package main

import (
	"github.com/foobaz/ansi"
	"io"
	"os"
)

func main() {
	var t ansi.Terminal
	io.Copy(&t, os.Stdin)
	t.WriteTo(os.Stdout)
}
