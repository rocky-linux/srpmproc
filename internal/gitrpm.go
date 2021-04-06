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
	"github.com/cavaliercoder/go-rpm"
)

// TODO: ugly hack, should create an interface
// since GitMode does not parse an RPM file, we just mimick
// the headers of an actual file to reuse RpmFile.Name()
func createPackageFile(name string) *rpm.PackageFile {
	return &rpm.PackageFile{
		Lead: rpm.Lead{},
		Headers: []rpm.Header{
			{},
			{
				Version:    0,
				IndexCount: 1,
				Length:     1,
				Indexes: []rpm.IndexEntry{
					{
						Tag:       1000,
						Type:      rpm.IndexDataTypeStringArray,
						Offset:    0,
						ItemCount: 1,
						Value:     []string{name},
					},
				},
			},
		},
	}
}
