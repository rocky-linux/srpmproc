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

package directives

import (
	"errors"
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	srpmprocpb "git.rockylinux.org/release-engineering/public/srpmproc/pb"
	"github.com/go-git/go-git/v5"
	"io/ioutil"
	"os"
	"path/filepath"
)

func add(cfg *srpmprocpb.Cfg, _ *data.ProcessData, _ *data.ModeData, patchTree *git.Worktree, pushTree *git.Worktree) error {
	for _, add := range cfg.Add {
		filePath := checkAddPrefix(filepath.Base(add.File))

		f, err := pushTree.Filesystem.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_OPEN_DESTINATION:%s", filePath))
		}

		fPatch, err := patchTree.Filesystem.OpenFile(add.File, os.O_RDONLY, 0644)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_OPEN_FROM:%s", add.File))
		}

		replacingBytes, err := ioutil.ReadAll(fPatch)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_READ_FROM:%s", add.File))
		}

		_, err = f.Write(replacingBytes)
		if err != nil {
			return errors.New(fmt.Sprintf("COULD_NOT_WRITE_DESTIONATION:%s", filePath))
		}
	}

	return nil
}
