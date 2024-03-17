package sizedbuf

import (
	"bufio"
	"io"
)

func New(writer io.Writer, limit int) *Sized {
	return &Sized{
		Writer: bufio.NewWriter(writer),
		limit:  limit,
	}
}

type Sized struct {
	*bufio.Writer
	size  int
	limit int
}

func (cw *Sized) Write(b []byte) (int, error) {
	n, err := cw.Writer.Write(b)
	if err != nil {
		return n, err
	}
	cw.size += n
	if cw.size >= cw.limit {
		err = cw.Writer.Flush()
		cw.size = 0
	}
	return n, err
}
