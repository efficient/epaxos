package fastrpc

import (
	"io"
)

type Serializable interface {
	Marshal(io.Writer)
	Unmarshal(io.Reader) error
	New() Serializable
}
