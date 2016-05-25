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

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
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

func (msg *Message) WriteTo(w io.Writer) (int64, error) {
	text := ""
	switch len(msg.Params) {
	case 0:
		text = msg.Command + "\r\n"
	case 1:
		text = fmt.Sprintf("%s :%s\r\n",
			msg.Command,
			msg.Params[len(msg.Params)-1])
	default:
		text = fmt.Sprintf("%s %s :%s\r\n",
			msg.Command,
			strings.Join(msg.Params[:len(msg.Params)-1], " "),
			msg.Params[len(msg.Params)-1])
	}
	if msg.Prefix != "" {
		text = fmt.Sprintf(":%s %s", msg.Prefix, text)
	}
	n, err := w.Write([]byte(text))
	return int64(n), err
}

// A Reader reads Messages from an io.Reader.
type Reader struct {
	scanner *bufio.Scanner
}

// Return a new Reader reading from r.
func NewReader(r io.Reader) *Reader {
	ret := &Reader{bufio.NewScanner(r)}
	ret.scanner.Buffer(make([]byte, MaxMessageLen), MaxMessageLen)
	return ret
}

// Read a message and return it.
//
// TODO: document errors. Right now just underlying IO errors.
//
// TODO: document the extent to which we validate the input.
func (r *Reader) ReadMessage() (*Message, error) {
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
		// No prefix. That byte is part of the command, so record it:
		output.WriteByte(c)
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
