package files

// An implementation of storage.Store that is backed by flat files.
//
// The on-disk format is as follows:
//
// * At the top level is a single directory, whose name is passed to OpenStore.
// * Each channel log is a file within this directory, whose name is the name of
//   the channel in utf-8 encoding, formatted as big-endian hexidecimal. This is
//   also the return value of:
//
//      fmt.Sprintf("%x", []byte(channelName))
//
// * Within each log, each message is a 16-bit little endian length field,
//   followed by the bytes of the message, including the trailing CRLF.
//   The length field does not include itself in the length it specifies.
//   Messages are stored in chronological order.
//
//   If on startup the end of the log contains a message whose length field
//   is larger than the remainder of the file, we assume an error occured when
//   writing the message, and discard it (truncating the file to the end of
//   the previous message).

import (
	"encoding/binary"
	"fmt"
	"os"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage"
)

type Store struct {
	path string
}

type channelLog struct {
	name      string
	file      *os.File
	endOffset int64
}

func OpenStore(path string) Store {
	return Store{path}
}

func (s Store) GetChannel(name string) (storage.ChannelLog, error) {
	filename := fmt.Sprintf("%s/%x", s.path, []byte(name))
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	endOffset, err := recoverLog(file)
	if err != nil {
		return nil, err
	}
	return &channelLog{
		name:      name,
		file:      file,
		endOffset: endOffset,
	}, nil
}

// Does on-startup log recovery (if needed) for the log file `file`,
// and returns the offset, to the the correct location to write the next
// log entry.
func recoverLog(file *os.File) (int64, error) {
	fi, err := file.Stat()
	if err != nil {
		return 0, err
	}
	fileSize := fi.Size()
	var (
		offset int64
		msgLen uint16
	)

	// find the end of the log:
	for {
		if fileSize-offset < 2 {
			// Most of the time this just means we're at the end of the
			// log, but this also covers the case where the length field
			// of the next entry is truncated (i.e. there is one more
			// byte).
			return offset, file.Truncate(offset)
		}
		err := binary.Read(file, binary.LittleEndian, &msgLen)
		if err != nil {
			return offset, err
		}
		if int64(msgLen) > fileSize-(offset+2) {
			// message is truncated; discard it.
			return offset, file.Truncate(offset)
		}
		offset, err = file.Seek(int64(msgLen), 1)
		if err != nil {
			return offset, err
		}
	}
}

func (cl *channelLog) Clear() error {
	cl.endOffset = 0
	return cl.file.Truncate(0)
}

func (cl *channelLog) LogMessage(msg *irc.Message) error {
	_, err := cl.file.Seek(cl.endOffset, 0)
	if err != nil {
		return err
	}
	err = binary.Write(cl.file, binary.LittleEndian, uint16(msg.Len()))
	if err != nil {
		return err
	}
	_, err = msg.WriteTo(cl.file)
	return err
}
