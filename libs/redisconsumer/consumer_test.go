package redisconsumer

import (
	must "github.com/stretchr/testify/require"
	"testing"
)

func TestChunkMessages(t *testing.T) {
	var testMessages []interface{}

	testChunks := chunkMessages(testMessages)
	must.Equal(t, 0, len(testChunks))

	for i := 0; i < 10000; i++ {
		testMessages = append(testMessages, i)
	}
	testChunks = chunkMessages(testMessages)
	must.Equal(t, 1, len(testChunks))
	must.Equal(t, 10000, len(testChunks[0]))

	for i := 10000; i < 20000; i++ {
		testMessages = append(testMessages, i)
	}
	testChunks = chunkMessages(testMessages)
	must.Equal(t, 2, len(testChunks))
	must.Equal(t, 10000, len(testChunks[0]))
	must.Equal(t, 10000, len(testChunks[1]))

	for i := 20000; i < 20500; i++ {
		testMessages = append(testMessages, i)
	}
	testChunks = chunkMessages(testMessages)
	must.Equal(t, 3, len(testChunks))
	must.Equal(t, 10000, len(testChunks[0]))
	must.Equal(t, 10000, len(testChunks[1]))
	must.Equal(t, 500, len(testChunks[2]))

	// test that we have everything we expect in the right order and chunks
	for i, chunk := range testChunks {
		if i == 0 {
			for ii := 0; ii < 10000; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii])
			}
		}
		if i == 1 {
			for ii := 0; ii < 10000; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii + 10000])
			}
		}
		if i == 2 {
			for ii := 0; ii < 500; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii + 20000])
			}
		}
	}
}
