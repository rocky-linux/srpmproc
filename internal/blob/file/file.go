package file

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type File struct {
	path string
}

func New(path string) *File {
	return &File{
		path: path,
	}
}

func (f *File) Write(path string, content []byte) {
	w, err := os.OpenFile(filepath.Join(f.path, path), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		log.Fatalf("could not open file: %v", err)
	}

	_, err = w.Write(content)
	if err != nil {
		log.Fatalf("could not write file to file: %v", err)
	}

	// Close, just like writing a file.
	if err := w.Close(); err != nil {
		log.Fatalf("could not close file writer to source: %v", err)
	}
}

func (f *File) Read(path string) []byte {
	r, err := os.OpenFile(filepath.Join(f.path, path), os.O_RDONLY, 0644)
	if err != nil {
		return nil
	}

	body, err := ioutil.ReadAll(r)
	if err != nil {
		return nil
	}

	return body
}

func (f *File) Exists(path string) bool {
	_, err := os.Stat(filepath.Join(f.path, path))
	return !os.IsNotExist(err)
}
