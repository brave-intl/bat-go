package utils

import "io"

// PanicCloser calls Close on the specified closer, panicing on error
func PanicCloser(c io.Closer) {
	err := c.Close()
	if err != nil {
		panic(err)
	}
}
