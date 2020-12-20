package main

import (
	"errors"
	"fmt"
	"github.com/mstg/srpmproc/internal"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
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

	name := strings.Split(metadataPath, ".")[1]

	metadataFile, err := os.Open(metadataPath)
	if err != nil {
		log.Fatalf("could not open metadata file: %v", err)
	}

	fileBytes, err := ioutil.ReadAll(metadataFile)
	if err != nil {
		log.Fatalf("could not read metadata file: %v", err)
	}

	cmd := exec.Command("git", "branch", "--show-current")
	branchByte, err := cmd.Output()
	if err != nil {
		log.Fatalf("could not get branch: %v", err)
	}
	branch := strings.TrimSuffix(string(branchByte), "\n")

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

		url := fmt.Sprintf("%s/%s/%s/%s", cdnUrl, name, branch, hash)
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

		hasher := internal.CompareHash(body, hash)
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
