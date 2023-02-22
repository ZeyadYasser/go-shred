package shred

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"os"
	"syscall"
)

type Opts struct {
	// Path of file to shred
	Path string
	// Number of shred iterations
	Iters int
	// Delete file after overwriting
	Delete bool
	// Do not round file sizes up to the next full block.
	// This is the default for non-regular files when using Shred(path).
	Exact bool
}

// Shred overwrites a file to hide its contents, and deletes it.
// The file is overwritten 3 times and deleted afterwards and
// rounds up to the next full block.
//
// For more control into how the file is shreded, look into ShredWithOpts(opts).
func Shred(path string) error {
	opts := Opts{
		Path:   path,
		Iters:  3,
		Delete: true,
		Exact:  false,
	}
	return ShredWithOpts(opts)
}

// ShredWithOpts overwrites a file to hide its contents, and optionally delete it.
// It gives more control on how the file is hidden from disk using Opts struct.
//
// For more info look into Opts struct.
func ShredWithOpts(opts Opts) error {
	f, err := os.OpenFile(opts.Path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("can't open %s: %w", opts.Path, err)
	}

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("can't get info of %s: %w", opts.Path, err)
	}
	if stat.IsDir() {
		return fmt.Errorf("%s should be a file not a directory", opts.Path)
	}

	fileSize := stat.Size()
	// To handle special files (e.g. /dev/sda)
	if !stat.Mode().IsRegular() {
		fileSize, err = f.Seek(0, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("can't seek file %s: %w", opts.Path, err)
		}
	}
	blockSize, err := getBlockSize(opts.Path)
	if err != nil {
		return err
	}
	// Round up to nearest block size multiple only
	// for regular files if the Exact flag is set.
	if !opts.Exact && stat.Mode().IsRegular() {
		fileSize += blockSize - 1 - (fileSize-1)%blockSize
	}

	// Initialize randomness source
	random, err := newRand()
	if err != nil {
		return fmt.Errorf("can't initialize randomness source: %w", err)
	}

	// Start shreding
	for i := 0; i < opts.Iters; i++ {
		if err := doIteration(f, fileSize, blockSize, random); err != nil {
			return err
		}
	}

	// Synchronize cached writes to persistent storage
	if err := f.Sync(); err != nil {
		return fmt.Errorf("can't sync %s: %w", opts.Path, err)
	}

	err = f.Close()
	if err != nil {
		return fmt.Errorf("can't close file %s: %w", opts.Path, err)
	}

	// Delete file if Delete flag is set.
	if opts.Delete {
		err := os.Remove(opts.Path)
		if err != nil {
			return fmt.Errorf("can't remove file %s: %w", opts.Path, err)
		}
	}

	return nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// doIteration does the actual overwritting of the file.
func doIteration(f *os.File, fileSize, blockSize int64, random *rand.Rand) error {
	// Start at begining of file
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("can't seek file: %w", err)
	}

	// Overwrite file
	buf := make([]byte, blockSize)
	offset := int64(0)
	for offset < fileSize {
		_, err := random.Read(buf)
		if err != nil {
			return fmt.Errorf("can't get random bytes: %w", err)
		}
		n, err := f.Write(buf[:min(blockSize, fileSize-offset)])
		if err != nil {
			return fmt.Errorf("can't write to file: %w", err)
		}
		offset += int64(n)
	}
	return nil
}

// getBlockSize determines the block size of a file.
func getBlockSize(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return -1, fmt.Errorf("can't get block size of %s: %w", path, err)
	}
	return stat.Bsize, nil
}

// newRand initializes a new randomness source.
//
// rand.Seed is now deperecated and this is the recommented new way.
//
// https://github.com/golang/go/issues/56319
func newRand() (*rand.Rand, error) {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return nil, fmt.Errorf("can't open /dev/urandom: %w", err)
	}

	buf := make([]byte, 8) // 8bytes to fit int64
	n, err := f.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("can't read from /dev/urandom: %w", err)
	}
	if n != 8 {
		return nil, fmt.Errorf("unexpected number of bytes read (%d bytes)", n)
	}

	seed := int64(binary.BigEndian.Uint64(buf))
	return rand.New(rand.NewSource(seed)), nil
}
