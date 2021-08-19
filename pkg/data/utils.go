// Copyright (c) 2021 The Srpmproc Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package data

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"github.com/go-git/go-billy/v5"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
)

func CopyFromFs(from billy.Filesystem, to billy.Filesystem, path string) {
	read, err := from.ReadDir(path)
	if err != nil {
		log.Fatalf("could not read dir: %v", err)
	}

	for _, fi := range read {
		fullPath := filepath.Join(path, fi.Name())

		if fi.IsDir() {
			_ = to.MkdirAll(fullPath, 0755)
			CopyFromFs(from, to, fullPath)
		} else {
			_ = to.Remove(fullPath)

			f, err := to.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fi.Mode())
			if err != nil {
				log.Fatalf("could not open file: %v", err)
			}

			oldFile, err := from.Open(fullPath)
			if err != nil {
				log.Fatalf("could not open from file: %v", err)
			}

			_, err = io.Copy(f, oldFile)
			if err != nil {
				log.Fatalf("could not copy from oldFile to new: %v", err)
			}
		}
	}
}

func IgnoredContains(a []*IgnoredSource, b string) bool {
	for _, val := range a {
		if val.Name == b {
			return true
		}
	}

	return false
}

func StrContains(a []string, b string) bool {
	for _, val := range a {
		if val == b {
			return true
		}
	}

	return false
}

// check if content and checksum matches
// returns the hash type if success else nil
func CompareHash(content []byte, checksum string) hash.Hash {
	var hashType hash.Hash

	switch len(checksum) {
	case 128:
		hashType = sha512.New()
		break
	case 64:
		hashType = sha256.New()
		break
	case 40:
		hashType = sha1.New()
		break
	case 32:
		hashType = md5.New()
		break
	default:
		return nil
	}

	hashType.Reset()
	_, err := hashType.Write(content)
	if err != nil {
		return nil
	}

	calculated := hex.EncodeToString(hashType.Sum(nil))
	if calculated != checksum {
		log.Printf("wanted checksum %s, but got %s", checksum, calculated)
		return nil
	}

	return hashType
}
