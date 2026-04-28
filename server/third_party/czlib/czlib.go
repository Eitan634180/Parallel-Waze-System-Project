package czlib

import (
	"compress/zlib"
	"io"
)

// NewReader provides the subset of the DataDog czlib API required by osmpbf.
func NewReader(r io.Reader) (io.ReadCloser, error) {
	return zlib.NewReader(r)
}
