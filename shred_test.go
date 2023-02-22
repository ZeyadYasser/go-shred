package shred

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testingDir = "/tmp/shred_testing"

func TestShredFileNotExist(t *testing.T) {
	filePath := path.Join(testingDir, "file_that_doesnt_exist")
	assert.Error(t, Shred(filePath), "Expected to fail for files that doesn't exist")
}

func TestShredDirectory(t *testing.T) {
	createTestFile([]byte{'a'}, t) // Make sure testingDir exists
	assert.Error(t, Shred(testingDir), "Expected to fail for directories")
}

func TestShredWithOptsDelete(t *testing.T) {
	data := []byte{'t', 'e', 's', 't'}
	filepath := createTestFile(data, t)
	err := ShredWithOpts(Opts{
		Path:   filepath,
		Iters:  1,
		Delete: true,
		Exact:  false,
	})
	assert.NoError(t, err)
	_, err = os.Stat(filepath)
	assert.True(t, errors.Is(err, os.ErrNotExist), "Expected file to be deleted")
}

func TestShredWithOptsNoDelete(t *testing.T) {
	data := []byte{'t', 'e', 's', 't'}
	filepath := createTestFile(data, t)
	err := ShredWithOpts(Opts{
		Path:   filepath,
		Iters:  1,
		Delete: false,
		Exact:  false,
	})
	assert.NoError(t, err)
	_, err = os.Stat(filepath)
	assert.False(t, errors.Is(err, os.ErrNotExist), "Expected file to not be deleted")
}

func TestShredWithOptsExact(t *testing.T) {
	data := []byte{'t', 'e', 's', 't'}
	filepath := createTestFile(data, t)
	err := ShredWithOpts(Opts{
		Path:   filepath,
		Iters:  1,
		Delete: false,
		Exact:  true,
	})
	assert.NoError(t, err)

	// Check file size after shredding
	stat, err := os.Stat(filepath)
	if err != nil {
		t.Fatalf("can't stat file %s", filepath)
	}
	assert.Equal(t, stat.Size(), int64(len(data)))
}

func TestShredWithOptsNoExact(t *testing.T) {
	data := []byte{'t', 'e', 's', 't'}
	filepath := createTestFile(data, t)
	err := ShredWithOpts(Opts{
		Path:   filepath,
		Iters:  1,
		Delete: false,
		Exact:  false,
	})
	assert.NoError(t, err)

	// Check file size after shredding
	stat, err := os.Stat(filepath)
	if err != nil {
		t.Fatalf("can't stat file %s", filepath)
	}
	var statfs syscall.Statfs_t
	if err := syscall.Statfs(filepath, &statfs); err != nil {
		t.Fatalf("can't stat fs which has %s", filepath)
	}

	// Expected size should be next nearest block size multiple.
	expectedSize := int64(len(data)) + statfs.Bsize - 1 - (int64(len(data)-1))%statfs.Bsize
	assert.Equal(t, stat.Size(), expectedSize)
}

func TestShredWithOptsRandomOverwrite(t *testing.T) {
	data := []byte{'t', 'e', 's', 't'}
	filepath := createTestFile(data, t)
	// First run
	err := ShredWithOpts(Opts{
		Path:   filepath,
		Iters:  1,
		Delete: false,
		Exact:  false,
	})
	assert.NoError(t, err)
	// Compare file before/after shredding
	newData1, err := ioutil.ReadFile(filepath)
	if err != nil {
		t.Fatalf("can't read file %s", filepath)
	}
	assert.NotEqual(t, data, newData1)

	// Second run
	err = ShredWithOpts(Opts{
		Path:   filepath,
		Iters:  1,
		Delete: false,
		Exact:  false,
	})
	assert.NoError(t, err)
	// Compare file before/after shredding
	newData2, err := ioutil.ReadFile(filepath)
	if err != nil {
		t.Fatalf("can't read file %s", filepath)
	}
	assert.NotEqual(t, newData1, newData2)
}

func TestShredBlockDevice(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root user is required")
	}

	cmd := exec.Command("/bin/sh", "-c", "losetup --help")
	cmd.Run()
	if !cmd.ProcessState.Success() {
		t.Skip("losetup tool is required")
	}

	data := make([]byte, 4096)
	for i := range data {
		data[i] = 'a' + uint8(i%2) // a or b
	}
	filepath := createTestFile(data, t)
	losetupCmd := fmt.Sprintf("losetup --find --show %s", filepath)
	loDev, err := exec.Command("/bin/sh", "-c", losetupCmd).Output()
	if err != nil {
		t.Fatalf("can't run losetup: %s", err)
	}
	loDevPath := strings.TrimSpace(string(loDev))
	t.Cleanup(func() {
		losetupCleanupCmd := fmt.Sprintf("losetup --detach %s", loDevPath)
		exec.Command("/bin/sh", "-c", losetupCleanupCmd).Run()
	})

	err = ShredWithOpts(Opts{
		Path:   loDevPath,
		Iters:  1,
		Delete: false,
		Exact:  false,
	})
	assert.NoError(t, err)
	// Compare file before/after shredding
	newData, err := ioutil.ReadFile(loDevPath)
	if err != nil {
		t.Fatalf("can't read file %s: %s", loDevPath, err)
	}
	assert.NotEqual(t, data, newData)
}

func TestShredPipe(t *testing.T) {
	// Create test directory if it doesn't exist.
	err := os.MkdirAll(testingDir, 0755)
	if err != nil {
		t.Fatalf("can't create test directory %s: %s", testingDir, err)
	}
	pipePath := path.Join(testingDir, "pipe")
	os.RemoveAll(pipePath) // Remove if exists
	err = syscall.Mkfifo(pipePath, 0777)
	if err != nil {
		t.Fatalf("can't create pipe %s: %s", pipePath, err)
	}
	t.Cleanup(func() {
		os.RemoveAll(pipePath)
	})

	// Write to pipe to avoid hanging shred when opening with O_WRONLY.
	f, err := os.OpenFile(pipePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("can't open pipe %s: %s", pipePath, err)
	}
	_, err = f.Write([]byte{'t', 'e', 's', 't'})
	if err != nil {
		t.Fatalf("can't write to pipe %s: %s", pipePath, err)
	}

	// Seeking is illegal for pipes, sockets, or FIFOs.
	// Check `man 2 lseek` errors section.
	err = Shred(pipePath)
	assert.Error(t, err)
}

func createTestFile(data []byte, t *testing.T) string {
	// Create test directory if it doesn't exist.
	err := os.MkdirAll(testingDir, 0755)
	if err != nil {
		t.Fatalf("can't create test directory %s: %s", testingDir, err)
	}

	// Create test file
	f, err := os.CreateTemp(testingDir, "test")
	if err != nil {
		t.Fatalf("can't create test file %s: %s", f.Name(), err)
	}

	// Write given data to test file
	n, err := f.Write(data)
	if err != nil {
		t.Fatalf("can't write to test file %s: %s", f.Name(), err)
	}
	if n != len(data) {
		t.Fatalf("wrote %d bytes only, %d bytes expected", n, len(data))
	}

	err = f.Close()
	if err != nil {
		t.Fatalf("can't close file %s: %s", f.Name(), err)
	}

	t.Cleanup(func() {
		// Shred could choose to not remove the file or fail to remove it.
		// This makes sure the proper test file cleanup is done regardless.
		os.RemoveAll(f.Name())
	})

	return f.Name()
}
