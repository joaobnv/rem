// This package deals with the reading of runes needed for parsers.
package rem

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

// File is a interface that deals with runes.
type File interface {
	// Next returns the rune at the current offset, unless the file is at EOF. It panics on error. It put the offset at the start of
	// the next rune, unless the file is at EOF. In the last case the offset remains unchanged.
	Next() (r rune, eof bool)

	// Previous returns the rune imediately before the current offset, unless the file is on the start of the file. It panics on error.
	// It put the offset at the start of the previous rune, unless the file is on the start of the file. In the
	// last case the offset remains unchanged.
	Previous() (r rune, onStart bool)

	// Consumed marks the bytes before offset as consumed. This means that the file client no longer needs
	// the file to provide access to these bytes. Some File types may free up memory or
	// decrease disk usage when this method is called. offset must be less than or equals the current offset
	// of the File.
	Consumed(offset int64)

	// Offset returns the current offset.
	Offset() int64

	// Close releases resources created by File.
	Close() error
}

// NewFile creates a new File that reads from data.
func NewFile(data []byte) File {
	return newBytesFile(data)
}

// NewFileFromString creates a new File that reads from str.
func NewFileFromString(str string) File {
	return newSeeker(strings.NewReader(str))
}

// NewFile creates a new File. memLimit is the maximum number of bytes in memory that can be allocated by the File.
// diskLimit is the maximum number of bytes in disk that can be allocated by the File. tempDir is the directory where
// disk files will be created. If tempDir is the empty string, the File uses the default directory for temporary files.
func NewFileFromReader(r io.Reader, memLimit, diskLimit int64, tempDir string) File {
	if buf, ok := r.(*bytes.Buffer); ok {
		if memLimit >= int64(buf.Len()) {
			memLimit = int64(buf.Len())
			diskLimit = 0
			tempDir = ""
		}
		return newReader(buf, memLimit, diskLimit, tempDir)
	}
	if s, ok := r.(io.ReadSeeker); ok {
		return newSeeker(s)
	}
	if ra, ok := r.(io.ReaderAt); ok {
		return newReaderAt(ra)
	}
	return newReader(r, memLimit, diskLimit, tempDir)
}

// reader is a File that uses a input that implements only io.Reader.
type reader struct {
	// r is the input.
	s *storage
}

// newReader creates a new reader. memLimit is the maximum number of bytes in memory that can be allocated by the reader.
// diskLimit is the maximum number of bytes in disk that can be allocated by the reader. tempDir is the directory where
// disk files will be created. If tempDir is the empty string, the reader uses the default directory for temporary files.
func newReader(r io.Reader, memLimit, diskLimit int64, tempDir string) *reader {
	return &reader{s: newStorage(r, memLimit, diskLimit, tempDir)}
}

// Next returns the rune at the current offset, unless r is at EOF. It panics on error. It put the offset at the start of
// the next rune, unless r is at EOF. In the last case the offset remains unchanged.
func (r *reader) Next() (rn rune, eof bool) {
	p := make([]byte, utf8.UTFMax)
	n, err := r.s.Read(p)
	if err == io.EOF { // when err == io.EOF the Read method read 0 bytes
		return 0, true
	} else if err != nil {
		panic(err)
	}

	rn, size := utf8.DecodeRune(p[:n])
	if rn == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	if size < n {
		r.s.seekRead(int64(-(n - size)))
	}

	return
}

// Previous returns the rune imediately before the current offset, unless r is on the start of the file. It panics on error.
// It put the offset at the start of the previous rune, unless r is on the start of the io.Reader. In the
// last case the offset remains unchanged.
func (r *reader) Previous() (rn rune, onStart bool) {
	if r.s.onStartRead() {
		return 0, true
	}
	b := make([]byte, 1)
	for !r.s.onStartRead() {
		r.s.seekRead(-1)

		_, err := r.s.Peek(b)
		if err != nil {
			panic(err)
		}

		if utf8.RuneStart(b[0]) {
			rn, _ = r.Peek()
			return
		}
	}
	panic(errors.New("invalid UTF-8 encoding"))
}

// Peek returns the next rune but dont advances the reader, this means that if Next is called it will return the same rune.
// Similarly for the eof.
func (r *reader) Peek() (rn rune, eof bool) {
	p := make([]byte, utf8.UTFMax)
	n, err := r.s.Peek(p)
	if err == io.EOF { // when err == io.EOF the Peek method read 0 bytes
		return 0, true
	} else if err != nil {
		panic(err)
	}

	rn, size := utf8.DecodeRune(p[:n])
	if rn == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	return
}

// Consumed marks the bytes before offset as consumed. This means that the reader client no longer needs
// that r provide access to these bytes. An attempt to access them has an undefined result. offset must be
// less than or equals the current offset of the reader.
func (r *reader) Consumed(offset int64) {
	r.s.Consumed(offset)
}

// Offset returns the current offset.
func (r *reader) Offset() int64 {
	return r.s.ReadOffset()
}

// Close releases resources created by storage.
func (r *reader) Close() error {
	return r.s.Close()
}

// storage handles the runes if if the input implements only io.Reader.
type storage struct {
	input io.Reader

	// memLimit is the max number of bytes that can be stored im memory. If this limit is exceeded then the
	// following bytes will be stored on disk, unless diskLimit is 0.
	memLimit int64

	// diskLimit is the max number of bytes that can be stored im disk. If this limit is exceeded then
	// attempts to store more bytes will panic.
	diskLimit int64

	// readOffset is the current offset for reading.
	readOffset int64

	// writeOffset is the current offset for writing.
	writeOffset int64

	// startOffset is the offset on the input where the start of mem is.
	startOffset int64

	// diskStart is the start position of data on disk
	diskStart int64

	// mem is where the bytes will be stored on memory
	mem []byte

	// disk is where the bytes will be stored on disk
	disk disk

	// tempDir is the directory for temporery files. If it is the empty string, storage uses the default directory for temporary files.
	tempDir string
}

// newStorage creates a new storage.
func newStorage(r io.Reader, memLimit, diskLimit int64, tempDir string) *storage {
	return &storage{input: r, memLimit: memLimit, diskLimit: diskLimit, tempDir: tempDir}
}

// Read implements io.Reader.
func (s *storage) Read(p []byte) (n int, err error) {
	n, err = s.Peek(p)
	if err != nil {
		return
	}
	s.readOffset += int64(n)
	return
}

// Peek reads up to len(p) bytes into p, but dont increment the read offset. It returns the number of bytes read (0 <= n <= len(p))
// and any error encountered
func (s *storage) Peek(p []byte) (n int, err error) {
	n = s.readFromMemory(p)
	if n == len(p) {
		return
	}
	s.seekRead(int64(n))

	n2, err := s.readFromDisk(p[n:])
	if err != nil && err != io.EOF {
		return
	} else if (n > 0 || n2 > 0) && err == io.EOF {
		err = nil
	}
	if n+n2 == len(p) {
		s.seekRead(-int64(n))
		return n + n2, err
	}
	s.seekRead(int64(n2))

	n3, err := s.readFromInput(p[n+n2:])
	if err != nil && err != io.EOF {
		return
	} else if (n > 0 || n2 > 0 || n3 > 0) && err == io.EOF {
		err = nil
	}

	s.seekRead(-int64(n + n2))

	return n + n2 + n3, err
}

// readFromMemory reads from memory. It dont increments the read offset.
func (s *storage) readFromMemory(p []byte) (n int) {
	avaliable := int64(len(s.mem)) - s.memoryOffset(s.readOffset)
	if avaliable >= int64(len(p)) {
		for i := range p {
			p[i] = s.mem[int64(i)+s.readOffset]
		}
		return len(p)
	} else if avaliable > 0 {
		for i := range avaliable {
			p[i] = s.mem[i+s.readOffset]
		}
		return int(avaliable)
	}
	return 0
}

// readFromDisk reads from disk. It dont increments the read offset.
func (s *storage) readFromDisk(p []byte) (n int, err error) {
	if s.disk == nil {
		return 0, io.EOF
	}
	return s.disk.ReadAt(p, s.diskOffset(s.readOffset))
}

// readFromInput reads from the input.
func (s *storage) readFromInput(p []byte) (n int, err error) {
	n, err = s.input.Read(p)
	if n == 0 {
		return
	}
	return s.Write(p[:n])
}

// Consumed marks the bytes before offset as consumed. This means that the storage client no longer needs
// that s provide access to these bytes. An attempt to access them has an undefined result. offset must be
// less than or equals the current read offset of the storage.
func (s *storage) Consumed(offset int64) {
	if offset > s.readOffset {
		panic(errors.New("invalid offset"))
	}
	if offset-s.startOffset >= s.memLimit {
		s.moveToMemory()
	}
}

// moveToMemory move bytes from s.disk to s.mem.
func (s *storage) moveToMemory() {
	if s.disk == nil {
		return
	}
	sr := io.NewSectionReader(s.disk, s.diskStart, s.memLimit)
	n, err := io.ReadFull(sr, s.mem)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		if err = s.disk.Truncate(0); err != nil {
			panic(err)
		}
		s.diskStart = 0
	} else if err != nil {
		panic(err)
	}

	if n < len(s.mem) {
		s.mem = s.mem[:n]
		s.mem = slices.Clip(s.mem)
	}

	if s.diskStart+int64(n) == s.diskOffset(s.writeOffset) {
		if err = s.disk.Truncate(0); err != nil {
			panic(err)
		}
		s.diskStart = 0
	} else {
		s.diskStart += int64(n)
	}

}

// seekRead seek the read offset from the current position.
func (s *storage) seekRead(offset int64) {
	s.readOffset += offset
	if s.readOffset < 0 {
		s.readOffset = 0
	}
}

// onStartRead reports whether the read offset is at the start of the input.
func (s *storage) onStartRead() bool {
	return s.readOffset == s.startOffset
}

// Write implements io.Writer.
func (s *storage) Write(p []byte) (n int, err error) {
	n = s.writeIntoMemory(p)
	if n == len(p) {
		return
	}
	nd, err := s.writeIntoDisk(p[n:])
	n += nd
	return
}

// writeIntoMemory writes len(p) bytes from p into s.mem. It returns the number of bytes written from p (0 <= n <= len(p)).
func (s *storage) writeIntoMemory(p []byte) (n int) {
	memOff := s.memoryOffset(s.writeOffset)
	avaliableMem := s.memLimit - memOff
	if avaliableMem >= int64(len(p)) {
		s.mem = append(s.mem, p...)
		s.writeOffset += int64(len(p))
		return len(p)
	}
	if avaliableMem > 0 {
		s.mem = append(s.mem, p[:avaliableMem]...)
		s.writeOffset += avaliableMem
		return int(avaliableMem)
	}
	return 0
}

// writeIntoDisk writes len(p) bytes from p into s.disk. It returns the number of bytes written from p (0 <= n <= len(p)).
func (s *storage) writeIntoDisk(p []byte) (n int, err error) {
	if s.disk == nil {
		if err = s.createDisk(); err != nil {
			return
		}
	}
	avaliableDisk := s.diskLimit - s.diskOffset(s.writeOffset)
	if avaliableDisk >= int64(len(p)) {
		n, err = s.disk.WriteAt(p, s.diskOffset(s.writeOffset))
		s.writeOffset += int64(n)
		return
	}
	return 0, errors.New("storage space has reached the limit")
}

// memoryOffset returns the offset from the start of s.mem corresponding to inputOffset.
func (s *storage) memoryOffset(inputOffset int64) int64 {
	result := inputOffset - s.startOffset
	if result < 0 {
		panic(errors.New("invalid offset"))
	}
	return result
}

// diskOffset returns the offset from the start of s.disk corresponding to inputOffset.
func (s *storage) diskOffset(inputOffset int64) int64 {
	result := inputOffset + s.diskStart - (s.startOffset + s.memLimit)
	if result < 0 {
		panic(errors.New("invalid offset"))
	}
	return result
}

// ReadOffset returns the current read offset.
func (s *storage) ReadOffset() int64 {
	return s.readOffset
}

// createDisk creates a temporary file for the s.disk.
func (s *storage) createDisk() (err error) {
	s.disk, err = os.CreateTemp(s.tempDir, "storage*.tmp")
	return
}

// Close removes the created temporary file if there is any, and frees up used memory.
func (s *storage) Close() error {
	s.mem = nil
	if s.disk == nil || s.disk == (*os.File)(nil) {
		return nil
	}
	s.disk.Close()
	err := os.Remove(s.disk.Name())
	s.disk = nil
	return err
}

// disk is an interface for a *os.File for enable tests to mock a *os.File. Note that only the methods needed by storage are in the interface.
type disk interface {
	io.ReaderAt
	io.WriterAt
	io.Closer
	Name() string
	Truncate(int64) error
}

// seeker is a File that uses a input that implements io.Reader and io.Seeker.
type seeker struct {
	// rs is the input.
	rs io.ReadSeeker
}

// newSeeker creates a new seeker.
func newSeeker(rs io.ReadSeeker) *seeker {
	return &seeker{rs}
}

// Next returns the rune at the current offset, unless s is at EOF. It panics on error. It put the offset at the start of
// the next rune, unless s is at EOF. In the last case the offset remains unchanged.
func (s *seeker) Next() (rn rune, eof bool) {
	p := make([]byte, utf8.UTFMax)
	n, err := io.ReadFull(s.rs, p)
	if err == io.EOF {
		return 0, true
	} else if err != nil && err != io.ErrUnexpectedEOF {
		panic(err)
	}

	rn, size := utf8.DecodeRune(p[:n])
	if rn == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	if size < n {
		s.rs.Seek(int64(-(n - size)), io.SeekCurrent)
	}

	return
}

// Previous returns the rune imediately before the current offset, unless s is on the start of the file. It panics on error.
// It put the offset at the start of the previous rune, unless s is on the start of the io.Reader. In the
// last case the offset remains unchanged.
func (s *seeker) Previous() (r rune, onStart bool) {
	if s.isOnStart() {
		return 0, true
	}

	offset := s.Offset()
	for {
		offset--
		if offset == -1 {
			break
		}
		if _, err := s.rs.Seek(offset, io.SeekStart); err != nil {
			panic(err)
		}

		b, _ := s.peekByte()

		if utf8.RuneStart(b) {
			r, _ = s.Peek()
			return
		}

	}
	panic(errors.New("invalid UTF-8 encoding"))
}

// Peek returns the next rune but dont advances the seeker, this means that if Next is called it will return the same rune.
// Similarly for the eof.
func (s *seeker) Peek() (r rune, eof bool) {
	p := make([]byte, utf8.UTFMax)
	n, err := io.ReadFull(s.rs, p)
	if err == io.EOF {
		return 0, true
	} else if err != nil && err != io.ErrUnexpectedEOF {
		panic(err)
	}

	r, size := utf8.DecodeRune(p[:n])
	if r == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	if _, err := s.rs.Seek(int64(-n), io.SeekCurrent); err != nil {
		panic(err)
	}

	return
}

// peekByte returns the next byte but dont advances the seeker.
func (s *seeker) peekByte() (b byte, eof bool) {
	p := make([]byte, 1)
	n, err := io.ReadFull(s.rs, p)
	if err == io.EOF {
		return 0, true
	} else if err != nil {
		panic(err)
	}

	if _, err := s.rs.Seek(int64(-n), io.SeekCurrent); err != nil {
		panic(err)
	}

	return p[0], false
}

// isOnStart reports whether the offset is at the start of the input.
func (s *seeker) isOnStart() bool {
	return s.Offset() == 0
}

// Consumed marks the bytes before offset as consumed. This means that the seeker client no longer needs
// that s provide access to these bytes. An attempt to access them has an undefined result. offset must be
// less than or equals the current offset of the seeker.
func (s *seeker) Consumed(offset int64) {
	if offset > s.Offset() {
		panic(errors.New("invalid offset"))
	}
}

// Offset returns the current offset.
func (s *seeker) Offset() int64 {
	offset, err := s.rs.Seek(0, io.SeekCurrent)
	if err != nil {
		panic(err)
	}
	return offset
}

// Close is a no-op. Always returns nil.
func (s *seeker) Close() error {
	return nil
}

// readerAt is a File that uses a input that implements io.ReaderAt.
type readerAt struct {
	// ra is the input.
	ra io.ReaderAt
	// offset is the current offset.
	offset int64
}

// newReaderAt creates a new readerAt.
func newReaderAt(ra io.ReaderAt) *readerAt {
	return &readerAt{ra: ra}
}

// Next returns the rune at the current offset, unless ra is at EOF. It panics on error. It put the offset at the start of
// the next rune, unless ra is at EOF. In the last case the offset remains unchanged.
func (ra *readerAt) Next() (r rune, eof bool) {
	p := make([]byte, utf8.UTFMax)
	n, err := ra.ra.ReadAt(p, ra.offset)
	if n == 0 && err == io.EOF {
		return 0, true
	} else if err != nil && err != io.EOF {
		panic(err)
	}

	r, size := utf8.DecodeRune(p[:n])
	if r == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	ra.offset += int64(size)

	return
}

// Previous returns the rune imediately before the current offset, unless ra is on the start of the input. It panics on error.
// It put the offset at the start of the previous rune, unless ra is on the start of the input. In the
// last case the offset remains unchanged.
func (ra *readerAt) Previous() (r rune, onStart bool) {
	if ra.offset == 0 {
		return 0, true
	}

	b := make([]byte, 1)
	for ra.offset != 0 {
		ra.offset--
		if _, err := ra.ra.ReadAt(b, ra.offset); err != nil {
			panic(err)
		}

		if utf8.RuneStart(b[0]) {
			r, _ = ra.Peek()
			return
		}
	}
	panic(errors.New("invalid UTF-8 encoding"))
}

// Peek returns the next rune but dont advances the reader, this means that if Next is called it will return the same rune.
// Similarly for the eof.
func (ra *readerAt) Peek() (rn rune, eof bool) {
	p := make([]byte, utf8.UTFMax)
	n, err := ra.ra.ReadAt(p, ra.offset)
	if n == 0 && err == io.EOF {
		return 0, true
	} else if err != nil && err != io.EOF {
		panic(err)
	}

	rn, size := utf8.DecodeRune(p[:n])
	if rn == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	return
}

// Consumed marks the bytes before offset as consumed. This means that the readerAt client no longer needs
// that ra provide access to these bytes. An attempt to access them has an undefined result. offset must be
// less than or equals the current offset of the readerAt.
func (ra *readerAt) Consumed(offset int64) {
	if offset > ra.Offset() {
		panic(errors.New("invalid offset"))
	}
}

// Offset returns the current offset.
func (ra *readerAt) Offset() int64 {
	return ra.offset
}

// Close is a no-op. Always returns nil.
func (ra *readerAt) Close() error {
	return nil
}

// bytesFile is a File that uses a byte slice as input.
type bytesFile struct {
	// b is the input.
	b []byte
	// offset is the current offset.
	offset int64
}

// newBytesFile creates a new bytesFile.
func newBytesFile(b []byte) *bytesFile {
	return &bytesFile{b: b}
}

// Next returns the rune at the current offset, unless bf is at EOF. It panics on error. It put the offset at the start of
// the next rune, unless bf is at EOF. In the last case the offset remains unchanged.
func (bf *bytesFile) Next() (rn rune, eof bool) {
	if bf.offset == int64(len(bf.b)) {
		return 0, true
	}

	rn, size := utf8.DecodeRune(bf.b[bf.offset:])
	if rn == utf8.RuneError && size == 1 {
		panic(errors.New("invalid UTF-8 encoding"))
	}

	bf.offset += int64(size)

	return
}

// Previous returns the rune imediately before the current offset, unless bf is on the start of the input. It panics on error.
// It put the offset at the start of the previous rune, unless bf is on the start of the input. In the
// last case the offset remains unchanged.
func (bf *bytesFile) Previous() (r rune, onStart bool) {
	if bf.offset == 0 {
		return 0, true
	}

	for {
		bf.offset--
		b := bf.b[bf.offset]
		if utf8.RuneStart(b) {
			r, _ = utf8.DecodeRune(bf.b[bf.offset:])
			return
		}
		if bf.offset == 0 {
			break
		}
	}
	panic(errors.New("invalid UTF-8 encoding"))
}

// Consumed marks the bytes before offset as consumed. This means that the readerAt client no longer needs
// that bf provide access to these bytes. An attempt to access them has an undefined result. offset must be
// less than or equals the current offset of the bytesFile.
func (bf *bytesFile) Consumed(offset int64) {
	if offset > bf.Offset() {
		panic(errors.New("invalid offset"))
	}
}

// Offset returns the current offset.
func (bf *bytesFile) Offset() int64 {
	return bf.offset
}

// Close is a no-op. Always returns nil.
func (bf *bytesFile) Close() error {
	return nil
}
