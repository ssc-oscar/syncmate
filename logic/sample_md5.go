package logic

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
)

type SampleMD5Result struct {
	Size   int64
	Digest string
}

// SampleMD5 divides the file into chunks and calculates the MD5 digest of the first 128 bytes of each chunk.
//
// Parameters:
//   - filePath: The path to the file
//   - skip: The number of bytes to skip from the beginning of the file. Defaults to 0.
//   - size: The number of bytes to use for hashing. If nil, the entire file (minus the skip) is considered.
//
// Returns:
//   - size: The size of the considered portion
//   - digest: The 16-character MD5 digest
//   - error: Any error encountered during processing
func SampleMD5(filePath string, skip int64, size int64) (*SampleMD5Result, error) {
	// Get file size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	fsize := fileInfo.Size()

	// Determine the size to consider
	var actualSize int64
	if size <= 0 {
		actualSize = fsize - skip
	} else {
		actualSize = size
	}

	// Validate parameters
	if skip+actualSize > fsize {
		return nil, fmt.Errorf("supplied size %dB > file size %dB", actualSize, fsize)
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Initialize MD5 hasher
	hasher := md5.New()

	// Hash all bytes if file is small
	if actualSize <= 4096 { // typical block size of ext4
		_, err = file.Seek(skip, io.SeekStart)
		if err != nil {
			return nil, err
		}

		buffer := make([]byte, actualSize)
		_, err = file.Read(buffer)
		if err != nil {
			return nil, err
		}

		hasher.Write(buffer)
		digest := fmt.Sprintf("%x", hasher.Sum(nil))
		return &SampleMD5Result{
			Size:   actualSize,
			Digest: digest[:16],
		}, nil
	}

	// A heuristic to find the optimal chunk size
	// Number of chunks is between 2 and 8, chunk size must be a power of 2
	chunkSize := int64(1) << (bitLength(actualSize/bitLength(actualSize)) + 2)
	numChunks := (actualSize - 256) / chunkSize // don't hash the same bytes twice

	// Seek to start position and read first 128 bytes
	_, err = file.Seek(skip, io.SeekStart)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 128)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}
	hasher.Write(buffer)

	// Hash chunks
	for i := int64(0); i < numChunks; i++ {
		_, err = file.Seek(chunkSize-128, io.SeekCurrent)
		if err != nil {
			return nil, err
		}

		_, err = file.Read(buffer)
		if err != nil {
			return nil, err
		}
		hasher.Write(buffer)
	}

	// Hash last 128 bytes
	_, err = file.Seek(skip+actualSize-128, io.SeekStart)
	if err != nil {
		return nil, err
	}

	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}
	hasher.Write(buffer)

	digest := fmt.Sprintf("%x", hasher.Sum(nil))
	return &SampleMD5Result{
		Size:   actualSize,
		Digest: digest[:16],
	}, nil
}

// bitLength returns the number of bits required to represent n
// This is equivalent to Python's int.bit_length()
func bitLength(n int64) int64 {
	if n == 0 {
		return 0
	}
	length := int64(0)
	for n > 0 {
		n >>= 1
		length++
	}
	return length
}
