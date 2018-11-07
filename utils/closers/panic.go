package closers

import "io"

// Panic calls Close on the specified closer, panicing on error
func Panic(c io.Closer) {
	err := c.Close()
	if err != nil {
		panic(err)
	}
}
