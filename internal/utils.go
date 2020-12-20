package internal

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

func copyFromFs(from billy.Filesystem, to billy.Filesystem, path string) {
	read, err := from.ReadDir(path)
	if err != nil {
		log.Fatalf("could not read dir: %v", err)
	}

	for _, fi := range read {
		fullPath := filepath.Join(path, fi.Name())

		if fi.IsDir() {
			_ = to.MkdirAll(fullPath, 0755)
			copyFromFs(from, to, fullPath)
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

func ignoredContains(a []*ignoredSource, b string) bool {
	for _, val := range a {
		if val.name == b {
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
