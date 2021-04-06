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

package internal

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	"github.com/cavaliercoder/go-cpio"
	"github.com/cavaliercoder/go-rpm"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SrpmMode struct{}

func (s *SrpmMode) RetrieveSource(pd *data.ProcessData) *data.ModeData {
	cmd := exec.Command("rpm2cpio", pd.RpmLocation)
	cpioBytes, err := cmd.Output()
	if err != nil {
		log.Fatalf("could not convert to cpio (maybe rpm2cpio is missing): %v", err)
	}

	// create in memory git repository
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		log.Fatalf("could not init git Repo: %v", err)
	}

	// read the rpm in cpio format
	buf := bytes.NewReader(cpioBytes)
	r := cpio.NewReader(buf)
	fileWrites := map[string][]byte{}
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			// end of cpio archive
			break
		}
		if err != nil {
			log.Fatalln(err)
		}

		bts, err := ioutil.ReadAll(r)
		if err != nil {
			log.Fatalf("could not copy file to virtual filesystem: %v", err)
		}
		fileWrites[hdr.Name] = bts
	}

	w, err := repo.Worktree()
	if err != nil {
		log.Fatalf("could not get Worktree: %v", err)
	}

	// create structure
	err = w.Filesystem.MkdirAll("SPECS", 0755)
	if err != nil {
		log.Fatalf("could not create SPECS dir in vfs: %v", err)
	}
	err = w.Filesystem.MkdirAll("SOURCES", 0755)
	if err != nil {
		log.Fatalf("could not create SOURCES dir in vfs: %v", err)
	}

	f, err := os.Open(pd.RpmLocation)
	if err != nil {
		log.Fatalf("could not open the file again: %v", err)
	}
	rpmFile, err := rpm.ReadPackageFile(f)
	if err != nil {
		log.Fatalf("could not read package, invalid?: %v", err)
	}

	var sourcesToIgnore []*data.IgnoredSource
	for _, source := range rpmFile.Source() {
		if strings.Contains(source, ".tar") {
			sourcesToIgnore = append(sourcesToIgnore, &data.IgnoredSource{
				Name:         source,
				HashFunction: sha256.New(),
			})
		}
	}

	branch := fmt.Sprintf("%s%d", pd.BranchPrefix, pd.Version)
	return &data.ModeData{
		Repo:            repo,
		Worktree:        w,
		RpmFile:         rpmFile,
		FileWrites:      fileWrites,
		Branches:        []string{branch},
		SourcesToIgnore: sourcesToIgnore,
	}
}

func (s *SrpmMode) WriteSource(_ *data.ProcessData, md *data.ModeData) {
	for fileName, contents := range md.FileWrites {
		var newPath string
		if filepath.Ext(fileName) == ".spec" {
			newPath = filepath.Join("SPECS", fileName)
		} else {
			newPath = filepath.Join("SOURCES", fileName)
		}

		mode := os.FileMode(0666)
		for _, file := range md.RpmFile.Files() {
			if file.Name() == fileName {
				mode = file.Mode()
			}
		}

		// add the file to the virtual filesystem
		// we will move it to correct destination later
		f, err := md.Worktree.Filesystem.OpenFile(newPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			log.Fatalf("could not create file %s: %v", fileName, err)
		}

		_, err = f.Write(contents)
		if err != nil {
			log.Fatalf("could not write to file %s: %v", fileName, err)
		}

		_ = f.Close()

		// don't add ignored file to git
		if ignoredContains(md.SourcesToIgnore, fileName) {
			continue
		}

		_, err = md.Worktree.Add(newPath)
		if err != nil {
			log.Fatalf("could not add source file: %v", err)
		}
	}

	// add sources to ignore (remote sources)
	gitIgnore, err := md.Worktree.Filesystem.Create(".gitignore")
	if err != nil {
		log.Fatalf("could not create .gitignore: %v", err)
	}
	for _, ignore := range md.SourcesToIgnore {
		line := fmt.Sprintf("SOURCES/%s\n", ignore)
		_, err := gitIgnore.Write([]byte(line))
		if err != nil {
			log.Fatalf("could not write line to .gitignore: %v", err)
		}
	}
	err = gitIgnore.Close()
	if err != nil {
		log.Fatalf("could not close .gitignore: %v", err)
	}
}

func (s *SrpmMode) PostProcess(_ *data.ModeData) {}

func (s *SrpmMode) ImportName(pd *data.ProcessData, _ *data.ModeData) string {
	return filepath.Base(pd.RpmLocation)
}
