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
	"github.com/rocky-linux/srpmproc/pkg/srpmproc"
	"github.com/spf13/cobra"
	"log"
	"os"
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
		log.Fatalf("could not get working directory: %v", err)
	}

	err = srpmproc.Fetch(os.Stdout, cdnUrl, wd)
	if err != nil {
		log.Fatal(err)
	}
}

func init() {
	root.AddCommand(fetch)
}
