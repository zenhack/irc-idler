// IRC protocol library
//
// Why IRC idler has it's own IRC protocol library: Basically, I looked around
// for stuff available for Go, and everything I found had at least one of
// these problems:
//
// * Unmaintained, or not obviously maintained
// * Insufficiently robust
// * Client-only
//
// This library's development is driven by the needs of IRC idler, but we hope
// it will be potentially useful to other applications. When it's mature, we
// may move it out of the IRC idler source tree.
//
// Goals
//
// * Be robust
// * Have the low-level components be usable even if the high-level stuff doesn't fit your
//   application. E.g. message parsing should work regardless of what you're building.
package irc

// This files defines the basic type for messages, interfaces for
// reading & writing them, and the obvious wrappers around io.Reader and
// io.Writer.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	MaxMessageLen = 512 // Maximum IRC message length, including CLRF.
)

// An IRC protocol message
type Message struct {
	Prefix  string // Empty string if absent.
	Command string
	Params  []string
}

// A Reader wraps the ReadMessage method
type Reader interface {
	// ReadMessage reads a message from the Reader.
	// This is required to be safe for concurrent readers.
	ReadMessage() (*Message, error)
}

// A Writer wraps the WriteMessage method
type Writer interface {
	// WriteMessage Writes a message to the Writer.
	// This is required to be safe for concurrent writers.
	WriteMessage(m *Message) error
}

type ReadWriter interface {
	Reader
	Writer
}

type ioReadWriter struct {
	Reader
	Writer
}

type ReadWriteCloser interface {
	Reader
	Writer
	io.Closer
}

type ioReadWriteCloser struct {
	ReadWriter
	io.Closer
}

func NewReadWriter(rw io.ReadWriter) ReadWriter {
	return ioReadWriter{NewReader(rw), NewWriter(rw)}
}

func NewReadWriteCloser(rwc io.ReadWriteCloser) ReadWriteCloser {
	return ioReadWriteCloser{
		ReadWriter: NewReadWriter(rwc),
		Closer:     rwc,
	}
}

type ioWriter struct {
	lock sync.Mutex
	w    io.Writer
}

func NewWriter(w io.Writer) Writer {
	return &ioWriter{w: w}
}

func (w *ioWriter) WriteMessage(m *Message) error {
	w.lock.Lock()
	defer w.lock.Unlock()
	_, err := m.WriteTo(w.w)
	return err
}

func (msg *Message) WriteTo(w io.Writer) (n int64, err error) {
	n = 0
	checkErr := func(sz int, e error) {
		if err != nil {
			err = e
		}
		n += int64(sz)
	}
	if msg.Prefix != "" {
		checkErr(fmt.Fprintf(w, ":%s ", msg.Prefix))
	}
	checkErr(fmt.Fprint(w, msg.Command))
	if len(msg.Params) != 0 {
		lastIndex := len(msg.Params) - 1
		for _, param := range msg.Params[:lastIndex] {
			checkErr(fmt.Fprintf(w, " %s", param))
		}
		lastParam := msg.Params[lastIndex]
		if strings.Contains(lastParam, " ") {
			checkErr(fmt.Fprintf(w, " :%s", lastParam))
		} else {
			checkErr(fmt.Fprintf(w, " %s", lastParam))
		}
	}
	checkErr(w.Write([]byte("\r\n")))
	return
}

func (msg *Message) String() string {
	buf := &bytes.Buffer{}
	msg.WriteTo(buf)
	return buf.String()
}

// Compare m1 and m2 for equality. We can't just use (==), as it
// doesn't work on string slices (Message.Params).
func (m1 *Message) Eq(m2 *Message) bool {
	if m1.Prefix != m2.Prefix || m1.Command != m2.Command {
		return false
	}
	if len(m1.Params) != len(m2.Params) {
		return false
	}
	for i := range m1.Params {
		if m1.Params[i] != m2.Params[i] {
			return false
		}
	}
	return true
}

// Return the length in bytes of the serialized form of the message.
func (m *Message) Len() int {
	total := 0
	if m.Prefix != "" {
		total += len(m.Prefix) + 2 // Leading ":" and trailing space.
	}
	total += len(m.Command)

	// Spaces before each parameter, including the space between the
	// command and the first parameter.
	total += len(m.Params)
	if len(m.Params) != 0 && strings.Contains(m.Params[len(m.Params)-1], " ") {
		// Colon before last argument:
		total += 1
	}
	for _, param := range m.Params {
		total += len(param)
	}
	total += 2 // trailing \r\n
	return total
}

// An ioReader reads Messages from an io.Reader.
type ioReader struct {
	lock    sync.Mutex
	scanner *bufio.Scanner
}

// Return a new Reader reading from r.
func NewReader(r io.Reader) Reader {
	ret := &ioReader{scanner: bufio.NewScanner(r)}
	ret.scanner.Buffer(make([]byte, MaxMessageLen), MaxMessageLen)
	return ret
}

// Read a message and return it.
//
// TODO: document errors. Right now just underlying IO errors.
//
// TODO: document the extent to which we validate the input.
func (r *ioReader) ReadMessage() (*Message, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	// We use bufio.Scanner to get each line, then parse the line from
	// a buffer.
	if !r.scanner.Scan() {
		err := r.scanner.Err()
		if err == nil {
			err = io.EOF
		}
		return nil, err
	}
	buf := bytes.NewBuffer(r.scanner.Bytes())
	return parseMessage(buf)
}

// Parse a message from a string. The string must contain exactly one message.
func ParseMessage(input string) (*Message, error) {
	r := NewReader(strings.NewReader(input))
	msg, err := r.ReadMessage()
	if err != nil {
		return nil, err
	}
	_, err = r.ReadMessage()
	if err != io.EOF {
		return nil, fmt.Errorf(
			"More than one message in string passed to parseMessage: %q",
			input)
	}
	return msg, nil
}

// parse the message in input
func parseMessage(input *bytes.Buffer) (*Message, error) {
	result := &Message{}
	output := &bytes.Buffer{}

	c, err := input.ReadByte()
	if err != nil {
		return nil, err
	}

	if c == ':' {
		// It's a prefix
		err = parseWord(output, input)
		if err != nil {
			return nil, err
		}
		result.Prefix = output.String()
		output.Reset()
	} else {
		// No prefix. That byte is part of the command, so put it back:
		input.UnreadByte()
	}

	err = parseWord(output, input)
	if err != nil {
		// no command
		return nil, err
	}
	result.Command = output.String()
	output.Reset()

	result.Params = []string{}
	for parseWord(output, input) == nil {
		result.Params = append(result.Params, output.String())
		output.Reset()
	}
	return result, nil
}

// Parse the next word (either space separated or a trailing argument starting
// with ':'), from input, and write it to output. May consume 1 additional
// character (typically a space), but otherwise leaves the remainder of input
// where it is.
func parseWord(output, input *bytes.Buffer) error {
	c, err := input.ReadByte()
	if err != nil {
		return err
	}
	if c == ':' {
		// End of the line; copy the whole thing:
		c, err = input.ReadByte()
		for err == nil {
			output.WriteByte(c)
			c, err = input.ReadByte()
		}
	} else {
		for err == nil && c != ' ' {
			output.WriteByte(c)
			c, err = input.ReadByte()
		}
	}
	if err == io.EOF {
		err = nil
	}
	return err
}
