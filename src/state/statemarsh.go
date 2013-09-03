package state

import (
	"encoding/binary"
	"io"
)

func (t *Command) Marshal(w io.Writer) {
	var b [8]byte
	bs := b[:8]
	bs = b[:1]
	b[0] = byte(t.Op)
	w.Write(bs)
	bs = b[:8]
	binary.LittleEndian.PutUint64(bs, uint64(t.K))
	w.Write(bs)
	binary.LittleEndian.PutUint64(bs, uint64(t.V))
	w.Write(bs)
}

func (t *Command) Unmarshal(r io.Reader) error {
	var b [8]byte
	bs := b[:8]
	bs = b[:1]
	if _, err := io.ReadFull(r, bs); err != nil {
		return err
	}
	t.Op = Operation(b[0])
	bs = b[:8]
	if _, err := io.ReadFull(r, bs); err != nil {
		return err
	}
	t.K = Key(binary.LittleEndian.Uint64(bs))
	if _, err := io.ReadFull(r, bs); err != nil {
		return err
	}
	t.V = Value(binary.LittleEndian.Uint64(bs))
	return nil
}

func (t *Key) Marshal(w io.Writer) {
	var b [8]byte
	bs := b[:8]
	binary.LittleEndian.PutUint64(bs, uint64(*t))
	w.Write(bs)
}

func (t *Value) Marshal(w io.Writer) {
	var b [8]byte
	bs := b[:8]
	binary.LittleEndian.PutUint64(bs, uint64(*t))
	w.Write(bs)
}

func (t *Key) Unmarshal(r io.Reader) error {
	var b [8]byte
	bs := b[:8]
	if _, err := io.ReadFull(r, bs); err != nil {
		return err
	}
	*t = Key(binary.LittleEndian.Uint64(bs))
	return nil
}

func (t *Value) Unmarshal(r io.Reader) error {
	var b [8]byte
	bs := b[:8]
	if _, err := io.ReadFull(r, bs); err != nil {
		return err
	}
	*t = Value(binary.LittleEndian.Uint64(bs))
	return nil
}
