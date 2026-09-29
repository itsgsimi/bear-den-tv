// The Wayland wire format, just enough of it for the Wayland adapter
// (wayland.go): 32-bit little-endian words, a header of object id then
// size<<16|opcode, strings with a length that counts the NUL and padding to
// four bytes, arrays with a byte length and the same padding. File
// descriptors are never sent or expected: none of the interfaces we bind
// carry them. Spec: https://wayland.freedesktop.org/docs/html/ch04.html;
// why hand-written: docs/decisions/0007-wayland-profile.md.

package wayland

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	headerSize = 8
	// maxMessage is libwayland's limit for one message, header included.
	maxMessage = 4096
)

var errShortMessage = errors.New("wayland: message body too short")

// message is one decoded wire message: the object it is addressed to (or
// sent from), the opcode and the raw argument bytes.
type message struct {
	object uint32
	opcode uint16
	body   []byte
}

// encoder builds the argument bytes of one request.
type encoder struct{ b []byte }

func (e *encoder) uint(v uint32) *encoder {
	e.b = binary.LittleEndian.AppendUint32(e.b, v)
	return e
}

func (e *encoder) string(s string) *encoder {
	e.uint(uint32(len(s) + 1))
	e.b = append(e.b, s...)
	e.b = append(e.b, 0)
	for len(e.b)%4 != 0 {
		e.b = append(e.b, 0)
	}
	return e
}

// decoder reads the arguments of one event in order. The first failure
// sticks: later reads return zero values and err reports it.
type decoder struct {
	b   []byte
	err error
}

func (d *decoder) uint() uint32 {
	if d.err != nil {
		return 0
	}
	if len(d.b) < 4 {
		d.err = errShortMessage
		return 0
	}
	v := binary.LittleEndian.Uint32(d.b)
	d.b = d.b[4:]
	return v
}

// bytes reads a length-prefixed, padded blob (the shape of string and array).
func (d *decoder) bytes() []byte {
	n := d.uint()
	if d.err != nil {
		return nil
	}
	padded := (int(n) + 3) &^ 3
	if n > maxMessage || len(d.b) < padded {
		d.err = errShortMessage
		return nil
	}
	v := d.b[:n]
	d.b = d.b[padded:]
	return v
}

func (d *decoder) string() string {
	b := d.bytes()
	if len(b) == 0 {
		return "" // a null string
	}
	if b[len(b)-1] != 0 {
		d.err = errors.New("wayland: string is not NUL-terminated")
		return ""
	}
	return string(b[:len(b)-1])
}

// array reads an array of 32-bit words (the only array type we receive:
// zwlr_foreign_toplevel_handle_v1.state).
func (d *decoder) array() []uint32 {
	b := d.bytes()
	if len(b)%4 != 0 {
		d.err = errors.New("wayland: array length is not a multiple of 4")
		return nil
	}
	out := make([]uint32, 0, len(b)/4)
	for i := 0; i+4 <= len(b); i += 4 {
		out = append(out, binary.LittleEndian.Uint32(b[i:]))
	}
	return out
}

// wire frames messages over one connection; writes are serialized.
type wire struct {
	rw  io.ReadWriteCloser
	br  *bufio.Reader
	wmu sync.Mutex
	hdr [headerSize]byte
}

func newWire(rw io.ReadWriteCloser) *wire {
	return &wire{rw: rw, br: bufio.NewReaderSize(rw, 2*maxMessage)}
}

// idle reports whether every byte received so far has been decoded: the
// end of what the compositor flushed together (one batch of events).
func (w *wire) idle() bool { return w.br.Buffered() == 0 }

// sendBatch writes several messages in one write, as a compositor flushes a
// batch of events (the tests' fake compositor uses it).
func (w *wire) sendBatch(msgs ...message) error {
	var all []byte
	for _, m := range msgs {
		b, err := frame(m.object, m.opcode, m.body)
		if err != nil {
			return err
		}
		all = append(all, b...)
	}
	w.wmu.Lock()
	defer w.wmu.Unlock()
	_, err := w.rw.Write(all)
	return err
}

// frame returns the complete bytes of one message.
func frame(object uint32, opcode uint16, body []byte) ([]byte, error) {
	size := headerSize + len(body)
	if size > maxMessage || len(body)%4 != 0 {
		return nil, fmt.Errorf("wayland: bad message size %d", size)
	}
	out := make([]byte, 0, size)
	out = binary.LittleEndian.AppendUint32(out, object)
	out = binary.LittleEndian.AppendUint32(out, uint32(size)<<16|uint32(opcode))
	return append(out, body...), nil
}

// send writes one request.
func (w *wire) send(object uint32, opcode uint16, body []byte) error {
	b, err := frame(object, opcode, body)
	if err != nil {
		return err
	}
	w.wmu.Lock()
	defer w.wmu.Unlock()
	_, err = w.rw.Write(b)
	return err
}

// read blocks for the next message. It is called from one goroutine only.
func (w *wire) read() (message, error) {
	if _, err := io.ReadFull(w.br, w.hdr[:]); err != nil {
		return message{}, err
	}
	obj := binary.LittleEndian.Uint32(w.hdr[0:])
	so := binary.LittleEndian.Uint32(w.hdr[4:])
	size := int(so >> 16)
	if size < headerSize || size > maxMessage || size%4 != 0 {
		return message{}, fmt.Errorf("wayland: bad message size %d from object %d", size, obj)
	}
	body := make([]byte, size-headerSize)
	if _, err := io.ReadFull(w.br, body); err != nil {
		return message{}, err
	}
	return message{object: obj, opcode: uint16(so & 0xffff), body: body}, nil
}

func (w *wire) close() error { return w.rw.Close() }
