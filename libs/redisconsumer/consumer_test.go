package redisconsumer

import (
	"testing"

	must "github.com/stretchr/testify/require"
)

func TestChunkMessagesEmpty(t *testing.T) {
	var testMessages []interface{}

	testChunks := chunkMessages(500, testMessages)
	must.Equal(t, 0, len(testChunks))
}

func TestChunkMessagesSingleChunk(t *testing.T) {
	var testMessages []interface{}

	for i := 0; i < 500; i++ {
		testMessages = append(testMessages, i)
	}
	testChunks := chunkMessages(500, testMessages)
	must.Equal(t, 1, len(testChunks))
	must.Equal(t, 500, len(testChunks[0]))

	// test that we have everything we expect in the right order and chunks
	for i, chunk := range testChunks {
		if i == 0 {
			for ii := 0; ii < 500; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii])
			}
		}
	}
}

func TestChunkMessagesMultipleChunks(t *testing.T) {
	var testMessages []interface{}
	for i := 0; i < 1000; i++ {
		testMessages = append(testMessages, i)
	}
	testChunks := chunkMessages(500, testMessages)
	must.Equal(t, 2, len(testChunks))
	must.Equal(t, 500, len(testChunks[0]))
	must.Equal(t, 500, len(testChunks[1]))

	for i, chunk := range testChunks {
		if i == 0 {
			for ii := 0; ii < 500; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii])
			}
		}
		if i == 1 {
			for ii := 0; ii < 500; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii + 500])
			}
		}
	}
}

func TestChunkMessagesOverflow(t *testing.T) {
	var testMessages []interface{}

	for i := 0; i < 1100; i++ {
		testMessages = append(testMessages, i)
	}
	testChunks := chunkMessages(500, testMessages)
	must.Equal(t, 3, len(testChunks))
	must.Equal(t, 500, len(testChunks[0]))
	must.Equal(t, 500, len(testChunks[1]))
	must.Equal(t, 100, len(testChunks[2]))

	for i, chunk := range testChunks {
		if i == 0 {
			for ii := 0; ii < 500; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii])
			}
		}
		if i == 1 {
			for ii := 0; ii < 500; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii + 500])
			}
		}
		if i == 2 {
			for ii := 0; ii < 100; ii++ {
				must.Equal(t, chunk[ii], testMessages[ii + 1000])
			}
		}
	}
}
