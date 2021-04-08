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

package main

import (
	"errors"
	"fmt"
	"git.rockylinux.org/release-engineering/public/srpmproc/internal/data"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var fetch = &cobra.Command{
	Use: "fetch",
	Run: runFetch,
}

var cdnUrl string

func init() {
	fetch.Flags().StringVar(&cdnUrl, "cdn-url", "", "Path to CDN")
	_ = fetch.MarkFlagRequired("cdn-url")
}

func runFetch(_ *cobra.Command, _ []string) {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	metadataPath := ""
	err = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".metadata") {
			if metadataPath != "" {
				return errors.New("multiple metadata files")
			}
			metadataPath = path
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	metadataFile, err := os.Open(metadataPath)
	if err != nil {
		log.Fatalf("could not open metadata file: %v", err)
	}

	fileBytes, err := ioutil.ReadAll(metadataFile)
	if err != nil {
		log.Fatalf("could not read metadata file: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: false,
		},
	}
	fileContent := strings.Split(string(fileBytes), "\n")
	for _, line := range fileContent {
		if strings.TrimSpace(line) == "" {
			continue
		}

		lineInfo := strings.Split(line, " ")
		hash := lineInfo[0]
		path := lineInfo[1]

		url := fmt.Sprintf("%s/%s", cdnUrl, hash)
		log.Printf("downloading %s", url)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Fatalf("could not create new http request: %v", err)
		}
		req.Header.Set("Accept-Encoding", "*")

		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("could not download dist-git file: %v", err)
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("could not read the whole dist-git file: %v", err)
		}
		err = resp.Body.Close()
		if err != nil {
			log.Fatalf("could not close body handle: %v", err)
		}

		hasher := data.CompareHash(body, hash)
		if hasher == nil {
			log.Fatal("checksum in metadata does not match dist-git file")
		}

		err = os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			log.Fatalf("could create all directories")
		}

		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("could not open file pointer: %v", err)
		}

		_, err = f.Write(body)
		if err != nil {
			log.Fatalf("could not copy dist-git file to in-tree: %v", err)
		}
		_ = f.Close()
	}
}

func init() {
	root.AddCommand(fetch)
}
