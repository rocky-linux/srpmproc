package internal

import (
	"fmt"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type remoteTarget struct {
	remote string
	when   time.Time
}

type remoteTargetSlice []remoteTarget

func (p remoteTargetSlice) Len() int {
	return len(p)
}

func (p remoteTargetSlice) Less(i, j int) bool {
	return p[i].when.Before(p[j].when)
}

func (p remoteTargetSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

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

	err = remote.Fetch(&git.FetchOptions{
		RefSpecs: []config.RefSpec{refspec},
		Tags:     git.AllTags,
		Force:    true,
	})
	if err != nil {
		log.Fatalf("could not fetch upstream: %v", err)
	}

	var branches remoteTargetSlice

	tagAdd := func(tag *object.Tag) error {
		if strings.HasPrefix(tag.Name, fmt.Sprintf("imports/%s%d", pd.ImportBranchPrefix, pd.Version)) {
			log.Printf("tag: %s", tag.Name)
			branches = append(branches, remoteTarget{
				remote: fmt.Sprintf("refs/tags/%s", tag.Name),
				when:   tag.Tagger.When,
			})
		}
		return nil
	}

	tagIter, err := repo.TagObjects()
	if err != nil {
		log.Fatalf("could not get tag objects: %v", err)
	}
	_ = tagIter.ForEach(tagAdd)

	if len(branches) == 0 {
		list, err := remote.List(&git.ListOptions{})
		if err != nil {
			log.Fatalf("could not list upstream: %v", err)
		}

		for _, ref := range list {
			if ref.Hash().IsZero() {
				continue
			}

			commit, err := repo.CommitObject(ref.Hash())
			if err != nil {
				log.Fatalf("could not get commit object: %v", err)
			}
			_ = tagAdd(&object.Tag{
				Name:   strings.TrimPrefix(string(ref.Name()), "refs/tags/"),
				Tagger: commit.Committer,
			})
		}
	}

	sort.Sort(branches)

	var sortedBranches []string
	for _, branch := range branches {
		sortedBranches = append(sortedBranches, branch.remote)
	}

	return &modeData{
		repo:       repo,
		worktree:   w,
		rpmFile:    createPackageFile(filepath.Base(pd.RpmLocation)),
		fileWrites: nil,
		branches:   sortedBranches,
	}
}

func (g *GitMode) WriteSource(pd *ProcessData, md *modeData) {
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
	if err != nil && err != git.NoErrAlreadyUpToDate {
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

		var body []byte

		if md.blobCache[hash] != nil {
			body = md.blobCache[hash]
			log.Printf("retrieving %s from cache", hash)
		} else {
			fromBlobStorage := pd.BlobStorage.Read(hash)
			if fromBlobStorage != nil {
				body = fromBlobStorage
				log.Printf("downloading %s from blob storage", hash)
			} else {
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

				body, err = ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Fatalf("could not read the whole dist-git file: %v", err)
				}
				err = resp.Body.Close()
				if err != nil {
					log.Fatalf("could not close body handle: %v", err)
				}
			}

			md.blobCache[hash] = body
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
			name:         path,
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
		_, err := md.worktree.Filesystem.Stat(source.name)
		if err == nil {
			err := md.worktree.Filesystem.Remove(source.name)
			if err != nil {
				log.Fatalf("could not remove dist-git file: %v", err)
			}
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
