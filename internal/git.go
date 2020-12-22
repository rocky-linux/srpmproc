package internal

import (
	"fmt"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

type GitMode struct{}

func (g *GitMode) RetrieveSource(pd *ProcessData) *modeData {
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		log.Fatalf("could not init git repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		log.Fatalf("could not get worktree: %v", err)
	}

	refspec := config.RefSpec("+refs/heads/*:refs/remotes/*")
	remote, err := repo.CreateRemote(&config.RemoteConfig{
		Name:  "upstream",
		URLs:  []string{pd.RpmLocation},
		Fetch: []config.RefSpec{refspec},
	})
	if err != nil {
		log.Fatalf("could not create remote: %v", err)
	}

	list, err := remote.List(&git.ListOptions{})
	if err != nil {
		log.Fatalf("could not list remote: %v", err)
	}

	var branches []string

	for _, ref := range list {
		log.Println(ref.String())
		name := string(ref.Name())
		if strings.HasPrefix(name, fmt.Sprintf("refs/tags/imports/c%d", pd.Version)) {
			branches = append(branches, name)
		}
	}

	// no tags? fetch branches
	if len(branches) == 0 {
		for _, ref := range list {
			name := string(ref.Name())
			prefix := fmt.Sprintf("refs/heads/c%d", pd.Version)
			if strings.HasPrefix(name, prefix) {
				branches = append(branches, name)
			}
		}
	}
	sort.Strings(branches)

	return &modeData{
		repo:       repo,
		worktree:   w,
		rpmFile:    createPackageFile(filepath.Base(pd.RpmLocation)),
		fileWrites: nil,
		branches:   branches,
	}
}

func (g *GitMode) WriteSource(md *modeData) {
	remote, err := md.repo.Remote("upstream")
	if err != nil {
		log.Fatalf("could not get upstream remote: %v", err)
	}

	var refspec config.RefSpec
	var branchName string

	if strings.HasPrefix(md.tagBranch, "refs/heads") {
		refspec = config.RefSpec(fmt.Sprintf("+%s:%s", md.tagBranch, md.tagBranch))
		branchName = strings.TrimPrefix(md.tagBranch, "refs/heads/")
	} else {
		match := tagImportRegex.FindStringSubmatch(md.tagBranch)
		branchName = match[2]
		refspec = config.RefSpec(fmt.Sprintf("+refs/heads/%s:%s", branchName, md.tagBranch))
	}
	log.Printf("checking out upstream refspec %s", refspec)
	err = remote.Fetch(&git.FetchOptions{
		RemoteName: "upstream",
		RefSpecs:   []config.RefSpec{refspec},
		Tags:       git.AllTags,
		Force:      true,
	})
	if err != nil {
		log.Fatalf("could not fetch upstream: %v", err)
	}

	err = md.worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(md.tagBranch),
		Force:  true,
	})
	if err != nil {
		log.Fatalf("could not checkout source from git: %v", err)
	}

	_, err = md.worktree.Add(".")
	if err != nil {
		log.Fatalf("could not add worktree: %v", err)
	}

	metadataFile, err := md.worktree.Filesystem.Open(fmt.Sprintf(".%s.metadata", md.rpmFile.Name()))
	if err != nil {
		log.Printf("warn: could not open metadata file, so skipping: %v", err)
		return
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

		lineInfo := strings.SplitN(line, " ", 2)
		hash := strings.TrimSpace(lineInfo[0])
		path := strings.TrimSpace(lineInfo[1])

		url := fmt.Sprintf("https://git.centos.org/sources/%s/%s/%s", md.rpmFile.Name(), branchName, hash)
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

		f, err := md.worktree.Filesystem.Create(path)
		if err != nil {
			log.Fatalf("could not open file pointer: %v", err)
		}

		hasher := CompareHash(body, hash)
		if hasher == nil {
			log.Fatal("checksum in metadata does not match dist-git file")
		}

		md.sourcesToIgnore = append(md.sourcesToIgnore, &ignoredSource{
			name:         filepath.Base(path),
			hashFunction: hasher,
		})

		_, err = f.Write(body)
		if err != nil {
			log.Fatalf("could not copy dist-git file to in-tree: %v", err)
		}
		_ = f.Close()
	}
}

func (g *GitMode) PostProcess(md *modeData) {
	for _, source := range md.sourcesToIgnore {
		err := md.worktree.Filesystem.Remove(filepath.Join("SOURCES", source.name))
		if err != nil {
			log.Fatalf("could not remove dist-git file: %v", err)
		}
	}

	_, err := md.worktree.Add(".")
	if err != nil {
		log.Fatalf("could not add git sources: %v", err)
	}
}

func (g *GitMode) ImportName(_ *ProcessData, md *modeData) string {
	if tagImportRegex.MatchString(md.tagBranch) {
		match := tagImportRegex.FindStringSubmatch(md.tagBranch)
		return match[3]
	}

	return strings.TrimPrefix(md.tagBranch, "refs/heads/")
}
