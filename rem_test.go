package rem

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// TestNew tests the functions that create File.
func TestNew(t *testing.T) {
	f := NewFile([]byte("test"))
	if _, ok := f.(*bytesFile); !ok {
		t.Errorf("expected *bytesFile, got %T", f)
	}

	f = NewFileFromString("test")
	if _, ok := f.(*seeker); !ok {
		t.Errorf("expected *seeker, got %T", f)
	}

	f = NewFileFromReader(bytes.NewBuffer([]byte("test")), 1<<20, 1<<34, ".")
	if r, ok := f.(*reader); !ok {
		t.Errorf("expected *seeker, got %T", f)
	} else {
		if r.s.memLimit != 4 {
			t.Errorf("expected memLimit = 4, got %d", r.s.memLimit)
		}
		if r.s.diskLimit != 0 {
			t.Errorf("expected diskLimit = 0, got %d", r.s.diskLimit)
		}
	}

	f = NewFileFromReader(strings.NewReader("test"), 1<<20, 1<<34, ".")
	if _, ok := f.(*seeker); !ok {
		t.Errorf("expected *seeker, got %T", f)
	}

	f = NewFileFromReader(newTestReaderAt("test"), 1<<20, 1<<34, ".")
	if _, ok := f.(*readerAt); !ok {
		t.Errorf("expected *readerAt, got %T", f)
	}

	f = NewFileFromReader(bufio.NewReader(strings.NewReader("test")), 1<<20, 1<<34, ".")
	if _, ok := f.(*reader); !ok {
		t.Errorf("expected *reader, got %T", f)
	}
}

// TestNew tests the File implementations.
func TestFile(t *testing.T) {
	files := []File{
		NewFile([]byte(strings.Repeat("abcd", 1<<8))),
		NewFileFromString(strings.Repeat("abcd", 1<<8)),
		NewFileFromReader(bytes.NewBuffer([]byte(strings.Repeat("abcd", 1<<8))), 1<<6, 1<<10, "."),
		NewFileFromReader(strings.NewReader(strings.Repeat("abcd", 1<<8)), 1<<6, 1<<10, "."),
		NewFileFromReader(newTestReaderAt(strings.Repeat("abcd", 1<<8)), 1<<6, 1<<10, "."),
		NewFileFromReader(bufio.NewReader(strings.NewReader(strings.Repeat("abcd", 1<<8))), 1<<6, 1<<10, "."),
	}

	defer func() {
		for i := range files {
			files[i].Close()
		}
	}()

	for _, f := range files {
		if f.Offset() != 0 {
			t.Errorf("expected offset = 0, got %d", f.Offset())
		}

		// Next
		for range (1 << 8) - 1 {
			for _, er := range "abcd" {
				r, eof := f.Next()
				if eof {
					t.Errorf("unexpected EOF")
				}
				if r != er {
					t.Errorf("expected %q, got %q", er, r)
				}
			}
		}
		for _, er := range "abcd" {
			r, eof := f.Next()
			if eof {
				t.Errorf("unexpected EOF")
			}
			if r != er {
				t.Errorf("expected %q, got %q", er, r)
			}
		}

		_, eof := f.Next()
		if !eof {
			t.Errorf("expected EOF")
		}

		f.Consumed(1 << 4)

		// Previous
		for range (1 << 4) - 1 {
			for _, er := range "dcba" {
				r, sof := f.Previous()
				if sof {
					t.Errorf("unexpected SOF")
				}
				if r != er {
					t.Errorf("expected %q, got %q", er, r)
				}
			}
		}
		for _, er := range "dcba" {
			r, sof := f.Previous()
			if sof {
				t.Errorf("unexpected SOF")
			}
			if r != er {
				t.Errorf("expected %q, got %q", er, r)
			}
		}
	}
}

// TestPanicReaderNext tests if the Next method of reader panics if the io.Reader returns a error diferent from io.EOF.
func TestPanicReaderNext(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader(errors.New("test"))
	f := NewFileFromReader(tr, 1, 1, ".")
	f.Next()
}

// TestReaderNextPanic tests if the Next method of reader panics if the io.Reader returns a error diferent from io.EOF.
func TestReaderNextPanic(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader(errors.New("test"))
	f := NewFileFromReader(tr, 1, 1, ".")
	f.Next()
}

// TestReaderNextPanic tests if the Next method of reader panics if the io.Reader returns invalid rune.
func TestReaderNextInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader([]byte{0xFF, 0xFF})
	f := NewFileFromReader(tr, 8, 0, ".")
	f.Next()
}

// TestPrevious tests the method Previous of File.
func TestPrevious(t *testing.T) {
	files := []File{
		NewFile([]byte("abcd")),
		NewFileFromString("abcd"),
		NewFileFromReader(bytes.NewBuffer([]byte("abcd")), 2, 2, "."),
		NewFileFromReader(strings.NewReader("abcd"), 2, 2, "."),
		NewFileFromReader(newTestReaderAt("abcd"), 2, 2, "."),
		NewFileFromReader(bufio.NewReader(strings.NewReader("abcd")), 2, 2, "."),
	}

	defer func() {
		for i := range files {
			files[i].Close()
		}
	}()

	for _, f := range files {
		for _, er := range "abcd" {
			r, eof := f.Next()
			if eof {
				t.Errorf("unexpected EOF")
			} else if r != er {
				t.Errorf("expected %q, got %q", er, r)
			}
		}
		_, eof := f.Next()
		if !eof {
			t.Errorf("expected EOF")
		}

		for _, er := range "dcba" {
			r, eof := f.Previous()
			if eof {
				t.Errorf("unexpected EOF")
			} else if r != er {
				t.Errorf("expected %q, got %q", er, r)
			}
		}
		_, sof := f.Previous()
		if !sof {
			t.Errorf("expected SOF")
		}
	}
}

// TestPanicConsumed tests if the Consumed method of reader panics if the offset is greather than the current offset
// the File.
func TestPanicConsumed(t *testing.T) {
	files := []File{
		NewFile([]byte("abcd")),
		NewFileFromString("abcd"),
		NewFileFromReader(bytes.NewBuffer([]byte("abcd")), 2, 2, "."),
		NewFileFromReader(strings.NewReader("abcd"), 2, 2, "."),
		NewFileFromReader(newTestReaderAt("abcd"), 2, 2, "."),
		NewFileFromReader(bufio.NewReader(strings.NewReader("abcd")), 2, 2, "."),
	}

	for _, f := range files {
		func(f File) {
			defer func() {
				err := recover()
				if err == nil {
					t.Errorf("panic expected")
					return
				}
				if msg := err.(error).Error(); msg != "invalid offset" {
					t.Errorf("expected error message with suffix %q, got %q", "invalid offset", msg)
				}
			}()

			f.Consumed(10)
		}(f)
	}
}

// TestPanicReaderPrevious tests if the Previous method of reader panics if the io.Reader returns a invalid rune.
func TestPanicReaderPrevious(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 4, 1, ".")
	f.Next()
	f.(*reader).s.mem[0] = 0b1000_0000
	f.Previous()
}

// TestPanicReachedLimit tests if the Next of reader method panics if the storage limit has reached.
func TestPanicReachedLimit(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "storage space has reached the limit" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 1, 1, ".")
	defer f.Close()
	f.Next()
}

// TestPanicReaderPrevious tests if the Previous method of reader panics if storage.Peek returns a error.
func TestPanicReaderStoragePeekError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); !strings.HasSuffix(msg, "file already closed") {
			t.Errorf("expected error message with suffix %q, got %q", "file already closed", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 0, 4, ".")
	f.Next()

	r := f.(*reader)
	r.s.disk.Close()
	os.Remove(r.s.disk.Name())
	f.Previous()
}

// TestReaderPeek tests the Peek method of reader.
func TestReaderPeek(t *testing.T) {
	tr := newTestReader([]byte{})
	f := NewFileFromReader(tr, 0, 0, ".").(*reader)
	_, eof := f.Peek()
	if !eof {
		t.Errorf("expected EOF")
	}
}

// TestPanicReaderPeekStoragePeekError tests if the Peek method of reader panics if storage.Peek returns a error.
func TestPanicReaderPeekStoragePeekError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); !strings.HasSuffix(msg, "file already closed") {
			t.Errorf("expected error message with suffix %q, got %q", "file already closed", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 0, 4, ".").(*reader)
	f.Next()

	f.s.disk.Close()
	os.Remove(f.s.disk.Name())
	f.Peek()
}

// TestReaderConsumed tests the Consumed method of reader.
func TestReaderConsumed(t *testing.T) {
	tr := newTestReader([]byte("abcd"))
	f := NewFileFromReader(tr, 2, 2, ".").(*reader)
	defer f.Close()
	f.Next()
	f.Next()
	f.Next()
	f.Next()
	f.Consumed(2)
	if bytes.Compare(f.s.mem, []byte("cd")) != 0 {
		t.Errorf("expected that s.mem = %q, got %q", []byte("cd"), f.s.mem)
	}
	fi, err := f.s.disk.(*os.File).Stat()
	if err != nil {
		t.Fatal(err)
	}
	if s := fi.Size(); s != 0 {
		t.Errorf("expected that the size of f.s.disk = 0, got %d", s)
	}

	tr2 := newTestReader([]byte("abcd"))
	f2 := NewFileFromReader(tr2, 3, 2, ".").(*reader)
	defer f2.Close()
	f2.Next()
	f2.Next()
	f2.Next()
	f2.Next()
	f2.Consumed(3)
	if bytes.Compare(f2.s.mem, []byte("d")) != 0 {
		t.Errorf("expected that s.mem = %q, got %q", []byte("d"), f2.s.mem)
	}
	fi2, err := f2.s.disk.(*os.File).Stat()
	if err != nil {
		t.Fatal(err)
	}
	if s := fi2.Size(); s != 0 {
		t.Errorf("expected that the size of f2.s.disk = 0, got %d", s)
	}

	tr3 := newTestReader([]byte("abcdef"))
	f3 := NewFileFromReader(tr3, 2, 4, ".").(*reader)
	defer f3.Close()
	f3.Next()
	f3.Next()
	f3.Next()
	f3.Consumed(2)
	if bytes.Compare(f3.s.mem, []byte("cd")) != 0 {
		t.Errorf("expected that s.mem = %q, got %q", []byte("cd"), f3.s.mem)
	}
	fi3, err := f3.s.disk.(*os.File).Stat()
	if err != nil {
		t.Fatal(err)
	}
	if s := fi3.Size(); s != 4 {
		t.Errorf("expected that the size of f3.s.disk = 0, got %d", s)
	}
	if f3.s.diskStart != 2 {
		t.Errorf("expected f3.s.diskStart = 2, got %d", f3.s.diskStart)
	}
}

// TestSeekReadLessThanReadOffset tests if the seekRead method of storage makes the readOffset equals 0 if the seek will make
// they less than 0.
func TestSeekReadLessThanReadOffset(t *testing.T) {
	tr := newTestReader([]byte("abcdef"))
	f := NewFileFromReader(tr, 6, 0, ".").(*reader)
	f.Next()
	f.s.seekRead(-5)
	if f.s.readOffset != 0 {
		t.Errorf("expected readOffset = 0, got %d", f.s.readOffset)
	}
}

// TestPanicReaderPeekInvalidRune tests if the Peek method of reader panics if the io.Reader returns a invalid rune.
func TestPanicReaderPeekInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 4, 1, ".").(*reader)
	f.Next()
	f.s.mem[f.Offset()] = 0b1000_0000
	f.Peek()
}

// TestNotPanicMoveToMemoryWithoutDisk tests if the moveToMemory method of storage not panics if the disk file is nil.
func TestNotPanicMoveToMemoryWithoutDisk(t *testing.T) {
	defer func() {
		err := recover()
		if err != nil {
			t.Errorf("unexpected panic")
			return
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 4, 0, ".").(*reader)
	f.s.moveToMemory()
}

// TestNotPanicMoveToMemoryFirstTruncateError tests if the moveToMemory method of storage panics if the first Truncate returns an error.
func TestNotPanicMoveToMemoryFirstTruncateError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 2, 2, ".").(*reader)
	defer f.Close()
	f.Next()
	f.s.disk = newTestDisk(f.s.disk.(*os.File), errors.New("test"), []any{io.EOF})
	f.s.moveToMemory()
}

// TestNotPanicMoveToMemorySecondTruncateError tests if the moveToMemory method of storage panics if the second Truncate returns an error.
func TestNotPanicMoveToMemorySecondTruncateError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 2, 2, ".").(*reader)
	defer f.Close()
	f.Next()
	f.s.disk = newTestDisk(f.s.disk.(*os.File), errors.New("test"), []any{[]byte("st")})
	f.s.moveToMemory()
}

// TestNotPanicMoveToMemoryReadError tests if the moveToMemory method of storage panics if Read returns an error.
func TestNotPanicMoveToMemoryReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 2, 2, ".").(*reader)
	defer f.Close()
	f.Next()
	f.s.disk = newTestDisk(f.s.disk.(*os.File), errors.New("test"), []any{errors.New("test")})
	f.s.moveToMemory()
}

// TestCreateDiskError tests if the reader panics if createDisk returns a error.
func TestCreatDiskError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 2, 2, "/////").(*reader) // not the invalid tempDir
	defer f.Close()
	f.Next()
}

// TestStorageInvalidMemoryOffset tests if the storage panics if memoryOffset is called with an invalid offset.
func TestStorageInvalidMemoryOffset(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}

		if msg := err.(error).Error(); msg != "invalid offset" {
			t.Errorf("expected error message %q, got %q", "invalid offset", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 2, 2, ".").(*reader)
	defer f.Close()
	f.Next()
	f.s.memoryOffset(-1)
}

// TestStorageInvalidDiskOffset tests if the storage panics if diskOffset is called with an invalid offset.
func TestStorageInvalidDiskOffset(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}

		if msg := err.(error).Error(); msg != "invalid offset" {
			t.Errorf("expected error message %q, got %q", "invalid offset", msg)
		}
	}()

	tr := newTestReader([]byte("test"))
	f := NewFileFromReader(tr, 2, 2, ".").(*reader)
	defer f.Close()
	f.Next()
	f.s.diskOffset(-1)
}

// TestPanicSeekerNextReadError tests if the Next method of seeker panics if the io.Reader returns error.
func TestPanicSeekerNextReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{errors.New("test")}, []any{errors.New("test")})
	f := NewFileFromReader(tr, 1, 0, ".")
	f.Next()
}

// TestPanicSeekerNextInvalidRune tests if the Next method of seeker panics if the io.Reader returns invalid rune.
func TestPanicSeekerNextInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte{0xFF, 0xFF}}, []any{[]byte{0xFF, 0xFF}})
	f := NewFileFromReader(tr, 8, 0, ".")
	f.Next()
}

// TestPanicSeekerPrevious tests if the Previous method of seeker panics if the io.Reader returns a invalid rune.
func TestPanicSeekerPrevious(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte("test")}, []any{[]byte("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	f.Next()
	tr.setData([]byte{0b1000_0000, 'e', 's', 't'})
	f.Previous()
}

// TestPanicSeekerPreviousSeekError tests if the Previous method of seeker panics if the io.Seeker returns
// a error on the Seek method.
func TestPanicSeekerPreviousSeekError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte("test")}, []any{[]byte("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	f.Next()
	tr.setSeekData(errors.New("test"), []byte("t"))
	f.Previous()
}

// TestPanicSeekerPreviousReadError tests if the Previous method of seeker panics if the io.Reader returns a error on the Read method.
func TestPanicSeekerPreviousReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte("test")}, []any{[]byte("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	f.Next()
	tr.setData(errors.New("test"), []byte("t"))
	f.Previous()
}

// TestSeekerPeek tests the Peek method of seeker.
func TestSeekerPeek(t *testing.T) {
	tr := newTestReadSeeker([]any{[]byte("t")}, []any{[]byte("t")})
	f := NewFileFromReader(tr, 4, 1, ".")
	s := f.(*seeker)
	r, eof := s.Peek()
	if eof {
		t.Errorf("unexpected EOF")
	}
	if r != 't' {
		t.Errorf("expected rune %q, got %q", 't', r)
	}
	s.Next()
	r, eof = s.Peek()
	if !eof {
		t.Errorf("expected EOF")
	}
}

// TestPanicSeekerPeekReadError tests if the Peek method of seeker panics if the io.Reader returns a error on the Read method.
func TestPanicSeekerPeekReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{errors.New("test")}, []any{errors.New("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	s := f.(*seeker)
	s.Peek()
}

// TestPanicSeekerPeekInvalidRune tests if the Peek method of seeker panics if the io.Reader returns invalid rune.
func TestPanicSeekerPeekInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte{0xFF, 0xFF}}, []any{[]byte{0xFF, 0xFF}})
	f := NewFileFromReader(tr, 4, 1, ".")
	s := f.(*seeker)
	s.Peek()
}

// TestPanicSeekerPeekSeekError tests if the Peek method of seeker panics if the io.Seeker returns a error on the Seek method.
func TestPanicSeekerPeekSeekError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte("test")}, []any{errors.New("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	s := f.(*seeker)
	s.Peek()
}

// TestSeekerPeekByteEOF tests if the peekByte method of seeker returns eof equals true when the Read of the io.ReadSeeker
// method returns io.EOF.
func TestSeekerPeekByteEOF(t *testing.T) {
	tr := newTestReadSeeker([]any{[]byte("")}, []any{})
	f := NewFileFromReader(tr, 4, 0, ".")
	s := f.(*seeker)
	_, eof := s.peekByte()
	if !eof {
		t.Errorf("expected EOF")
	}
}

// TestPanicSeekerPeekByteReadError tests if the peekByte method of seeker panics if the io.Reader returns a error on the Read method.
func TestPanicSeekerPeekByteReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{errors.New("test")}, []any{errors.New("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	s := f.(*seeker)
	s.peekByte()
}

// TestPanicSeekerPeekByteSeekError tests if the peekByte method of seeker panics if the io.Seeker returns a error on the Seek method.
func TestPanicSeekerPeekByteSeekError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReadSeeker([]any{[]byte("test")}, []any{errors.New("test")})
	f := NewFileFromReader(tr, 4, 1, ".")
	s := f.(*seeker)
	s.peekByte()
}

// TestPanicReaderAtNextReadError tests if the Next method of readerAt panics if the io.ReaderAt returns error.
func TestPanicReaderAtNextReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReaderAt("test", errors.New("test"))
	f := NewFileFromReader(tr, 1, 0, ".")
	f.Next()
}

// TestPanicReaderAtNextInvalidRune tests if the Next method of readerAt panics if the io.ReaderAt returns an invalid rune.
func TestPanicReaderAtNextInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReaderAt(string([]byte{0xFF, 0xFF}))
	f := NewFileFromReader(tr, 8, 0, ".")
	f.Next()
}

// TestPanicReaderAtPreviousReadAtError tests if the Previous method of seeker panics if the io.ReadAt returns
// a error on the ReadAt method.
func TestPanicReaderAtPreviousReadAtError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReaderAt("test", nil, errors.New("test"))
	f := NewFileFromReader(tr, 4, 0, ".")
	f.Next()
	f.Previous()
}

// TestPanicReaderAtPrevious tests if the Previous method of readerAt panics if the io.ReaderAt returns a invalid rune.
func TestPanicReaderAtPrevious(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReaderAt("test", nil, []byte{0b1000_0000})
	f := NewFileFromReader(tr, 4, 0, ".")
	f.Next()
	f.Previous()
}

// TestPanicReaderAtPeekEOF tests if the Peek method of readerAt returns eof equals true when the ReadAt of the io.ReadAt
// method returns io.EOF.
func TestPanicReaderAtPeekEOF(t *testing.T) {
	tr := newTestReaderAt("", nil)
	f := NewFileFromReader(tr, 4, 0, ".")
	ra := f.(*readerAt)
	_, eof := ra.Peek()
	if !eof {
		t.Errorf("expected EOF")
	}
}

// TestPanicReaderAtPeekReadError tests if the Peek method of readerAt panics if the io.ReaderAt returns an error.
func TestPanicReaderAtPeekReadError(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "test" {
			t.Errorf("expected error message %q, got %q", "test", msg)
		}
	}()

	tr := newTestReaderAt("test", errors.New("test"))
	f := NewFileFromReader(tr, 1, 0, ".")
	ra := f.(*readerAt)
	ra.Peek()
}

// TestPanicReaderAtPeekInvalidRune tests if the Peek method of readerAt panics if the io.ReaderAt returns an invalid rune.
func TestPanicReaderAtPeekInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	tr := newTestReaderAt(string([]byte{0xFF, 0xFF}))
	f := NewFileFromReader(tr, 8, 0, ".")
	ra := f.(*readerAt)
	ra.Peek()
}

// TestPanicBytesNextInvalidRune tests if the Next method of bytesFile panics if the next rune of the byte slice is invalid.
func TestPanicBytesNextInvalidRune(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	f := NewFile([]byte{0xFF, 0xFF})
	f.Next()
}

// TestPanicBytesPrevious tests if the Previous method of bytesFile panics if the byte slice previuos rune is invalid.
func TestPanicBytesPrevious(t *testing.T) {
	defer func() {
		err := recover()
		if err == nil {
			t.Errorf("panic expected")
			return
		}
		if msg := err.(error).Error(); msg != "invalid UTF-8 encoding" {
			t.Errorf("expected error message %q, got %q", "invalid UTF-8 encoding", msg)
		}
	}()

	f := NewFile([]byte("test"))
	f.Next()
	f.(*bytesFile).b = []byte{0b1000_0000, 'e', 's', 't'}
	f.Previous()
}

// testReaderAt is a io.ReadAt for tests.
type testReaderAt struct {
	r *strings.Reader
	// readAtResult contains what must be the result of the calls to ReadAt. It can contains byte slices, nils and errors.
	// A nil means that the method must call the ReadAt of r.
	readAtResult []any
	// callNumber is the number of calls to ReadAt.
	callNumber int
}

// newTestReaderAt creates a new newTestReaderAt.
func newTestReaderAt(str string, reatAtResult ...any) *testReaderAt {
	return &testReaderAt{r: strings.NewReader(str), readAtResult: reatAtResult}
}

// Read implements io.Reader.
func (tra *testReaderAt) Read(p []byte) (int, error) {
	return tra.r.Read(p)
}

// ReadAt implements io.ReaderAt.
func (tra *testReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if tra.callNumber >= len(tra.readAtResult) || tra.readAtResult[tra.callNumber] == nil {
		tra.callNumber++
		return tra.r.ReadAt(p, off)
	} else if d, ok := tra.readAtResult[tra.callNumber].([]byte); ok {
		n := copy(p, d)
		tra.callNumber++
		if n < len(p) {
			return n, io.EOF
		}
		return n, nil
	}
	err := tra.readAtResult[tra.callNumber].(error)
	tra.callNumber++
	return 0, err
}

// testReader is a io.Reader for tests.
type testReader struct {
	// data contains the things that the reader can return, it can be byte slices, errors, or ints. A int indicates
	// that the method should do nothing.
	data []any
	// pos is the current position on data
	pos int
	// offset is the current offset on the byte slice at pos in data
	offset int
}

// newTestReader creates a new testReader
func newTestReader(data ...any) *testReader {
	return &testReader{data: data}
}

// Read implements io.Reader.
func (tr *testReader) Read(p []byte) (n int, err error) {
	for {
		if tr.pos == len(tr.data) {
			return n, io.EOF
		}
		d := tr.data[tr.pos]
		if b, ok := d.([]byte); ok {
			if tr.offset < len(b) {
				copied := copy(p[n:], b[tr.offset:])
				n += copied
				tr.offset += copied
				if n == len(p) {
					return
				}
			} else {
				tr.offset = 0
				tr.pos++
			}
		} else if err, ok = d.(error); ok {
			return
		} else if _, ok = d.(int); ok {

		} else {
			panic(fmt.Errorf("invalid type: %T", d))
		}
	}
}

func (tr *testReader) setData(data ...any) {
	tr.data = data
}

// testReader is a io.ReadSeeker for tests.
type testReadSeeker struct {
	*testReader
	// seekData contains the things that the seeker can return, it can be errors, or nils.
	seekData []any
	// seekPos is the current position on seekData
	seekPos int
}

// newTestReadSeeker creates a new testReadSeeker.
func newTestReadSeeker(readData, seekData []any) *testReadSeeker {
	return &testReadSeeker{testReader: newTestReader(readData...), seekData: seekData}
}

// Seek implements io.Seeker.
func (trs *testReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if err, ok := trs.seekData[trs.seekPos].(error); ok {
		trs.seekPos++
		return 0, err
	}
	if whence == io.SeekStart {
		if offset < 0 {
			panic(errors.New("invalid offset"))
		}
		var curOff int64
		for i, d := range trs.data {
			if err, ok := d.(error); ok {
				if curOff == offset {
					return curOff, err
				}
				continue
			} else if _, ok = d.(int); ok {
				if curOff == offset {
					return offset, nil
				}
				continue
			}
			b := d.([]byte)
			if offset-curOff < int64(len(b)) {
				trs.pos = i
				trs.offset = int(offset - curOff)
				return offset, nil
			}
			curOff += int64(len(b))
		}
		if curOff == offset {
			return offset, nil
		}
		return offset, io.EOF
	} else if whence == io.SeekCurrent {
		curOff, err := trs.currentOffset()
		if err != nil {
			return curOff, err
		}
		return trs.Seek(curOff+offset, io.SeekStart)
	} else {
		endOff, err := trs.endOffset()
		if err != nil {
			return endOff, err
		}
		return trs.Seek(endOff+offset, io.SeekStart)
	}
}

func (trs *testReadSeeker) setSeekData(seekData ...any) {
	trs.seekData = seekData
}

// currentOffset returns the current offset.
func (trs *testReadSeeker) currentOffset() (int64, error) {
	var curOff int64
	for i, d := range trs.data {
		if _, ok := d.(error); ok {
			continue
		} else if _, ok = d.(int); ok {
			continue
		}
		if i == trs.pos {
			curOff += int64(trs.offset)
			break
		}
		curOff += int64(len(d.([]byte)))
	}
	return curOff, nil
}

// endOffset returns the offset of the end.
func (trs *testReadSeeker) endOffset() (int64, error) {
	var curOff int64
	for _, d := range trs.data {
		if err, ok := d.(error); ok {
			return curOff, err
		}
		curOff += int64(len(d.([]byte)))
	}
	return curOff, nil
}

// testDisk is a disk that for tests.
type testDisk struct {
	*os.File
	truncate error
	data     []any
	// pos is the current position on data
	pos int
	// offset is the current offset on the byte slice at pos int data
	offset int64
}

// newTestDisk creates a new disk for tests. truncate is what the Truncate method must return.
// data is the things that the ReadAt method must return, it can be byte slices or errors.
func newTestDisk(f *os.File, truncate error, data []any) disk {
	return &testDisk{File: f, truncate: truncate, data: data}
}

// Truncate implements disk and returns a error when called.
func (td *testDisk) Truncate(size int64) error {
	return td.truncate
}

func (td *testDisk) ReadAt(p []byte, off int64) (n int, err error) {
	if err = td.toOffset(off); err != nil {
		return
	}
	for {
		if td.pos == len(td.data) {
			return n, io.EOF
		}
		d := td.data[td.pos]
		if b, ok := d.([]byte); ok {
			if td.offset < int64(len(b)) {
				copied := copy(p[n:], b[td.offset:])
				n += copied
				if n == len(p) {
					td.offset += int64(copied)
					return
				}
			} else {
				td.offset = 0
				td.pos++
			}
		} else if err, ok = d.(error); ok {
			return
		} else {
			panic(fmt.Errorf("invalid type: %T", d))
		}
	}
}

func (td *testDisk) toOffset(off int64) (err error) {
	offset := int64(0)
	td.pos = 0
	for {
		if td.pos == len(td.data) {
			return io.EOF
		}
		d := td.data[td.pos]
		if b, ok := d.([]byte); ok {
			if off-offset < int64(len(b)) {
				td.offset = off - offset
				return nil
			}
			offset += int64(len(b))
			td.pos++
		} else if e, ok := d.(error); ok {
			return e
		} else {
			panic(fmt.Errorf("invalid type: %T", d))
		}
	}
}
