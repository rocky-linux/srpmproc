package internal

import (
	"encoding/hex"
	"fmt"
	"github.com/cavaliercoder/go-rpm"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/mstg/srpmproc/internal/blob"
	"hash"
	"io/ioutil"
	"log"
	"regexp"
	"strings"
	"time"
)

var tagImportRegex = regexp.MustCompile("refs/tags/(imports/(.*)/(.*))")

type ProcessData struct {
	RpmLocation       string
	UpstreamPrefix    string
	SshKeyLocation    string
	SshUser           string
	Version           int
	GitCommitterName  string
	GitCommitterEmail string
	Mode              int
	ModulePrefix      string
	Authenticator     *ssh.PublicKeys
	Importer          ImportMode
	BlobStorage       blob.Storage
}

type ignoredSource struct {
	name         string
	hashFunction hash.Hash
}

type modeData struct {
	repo            *git.Repository
	worktree        *git.Worktree
	rpmFile         *rpm.PackageFile
	fileWrites      map[string][]byte
	tagBranch       string
	pushBranch      string
	branches        []string
	sourcesToIgnore []*ignoredSource
}

// ProcessRPM checks the RPM specs and discards any remote files
// This functions also sorts files into directories
// .spec files goes into -> SPECS
// metadata files goes to root
// source files goes into -> SOURCES
// all files that are remote goes into .gitignore
// all ignored files' hash goes into .{name}.metadata
func ProcessRPM(pd *ProcessData) {
	md := pd.Importer.RetrieveSource(pd)

	sourceRepo := *md.repo
	sourceWorktree := *md.worktree

	for _, branch := range md.branches {
		md.repo = &sourceRepo
		md.worktree = &sourceWorktree
		md.tagBranch = branch
		md.sourcesToIgnore = []*ignoredSource{}

		rpmFile := md.rpmFile
		// create new repo for final dist
		repo, err := git.Init(memory.NewStorage(), memfs.New())
		if err != nil {
			log.Fatalf("could not create new dist repo: %v", err)
		}
		w, err := repo.Worktree()
		if err != nil {
			log.Fatalf("could not get dist worktree: %v", err)
		}

		if !tagImportRegex.MatchString(md.tagBranch) {
			log.Fatal("import tag invalid")
		}

		match := tagImportRegex.FindStringSubmatch(md.tagBranch)
		md.pushBranch = "rocky" + strings.TrimPrefix(match[2], "c")

		// create a new remote
		remoteUrl := fmt.Sprintf("%s/dist/%s.git", pd.UpstreamPrefix, rpmFile.Name())
		log.Printf("using remote: %s", remoteUrl)
		refspec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", md.pushBranch, md.pushBranch))
		log.Printf("using refspec: %s", refspec)

		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			log.Fatalf("could not create remote: %v", err)
		}

		err = repo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{refspec},
			Auth:       pd.Authenticator,
		})

		refName := plumbing.NewBranchReferenceName(md.pushBranch)
		log.Printf("set reference to ref: %s", refName)

		if err != nil {
			h := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
			if err := repo.Storer.CheckAndSetReference(h, nil); err != nil {
				log.Fatalf("could not set reference: %v", err)
			}
		} else {
			err = w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewRemoteReferenceName("origin", md.pushBranch),
				Force:  true,
			})
			if err != nil {
				log.Fatalf("could not checkout: %v", err)
			}
		}

		pd.Importer.WriteSource(md)

		copyFromFs(md.worktree.Filesystem, w.Filesystem, ".")
		md.repo = repo
		md.worktree = w

		executePatches(pd, md)

		// get ignored files hash and add to .{name}.metadata
		metadataFile := fmt.Sprintf(".%s.metadata", rpmFile.Name())
		metadata, err := w.Filesystem.Create(metadataFile)
		if err != nil {
			log.Fatalf("could not create metadata file: %v", err)
		}
		for _, source := range md.sourcesToIgnore {
			sourcePath := "SOURCES/" + source.name
			sourceFile, err := w.Filesystem.Open(sourcePath)
			if err != nil {
				log.Fatalf("could not open ignored source file %s: %v", sourcePath, err)
			}
			sourceFileBts, err := ioutil.ReadAll(sourceFile)
			if err != nil {
				log.Fatalf("could not read the whole of ignored source file: %v", err)
			}

			path := fmt.Sprintf("%s-%s/%s", rpmFile.Name(), md.pushBranch, source.name)
			pd.BlobStorage.Write(path, sourceFileBts)
			log.Printf("wrote %s to blob storage", path)

			source.hashFunction.Reset()
			_, err = source.hashFunction.Write(sourceFileBts)
			if err != nil {
				log.Fatalf("could not write bytes to hash function: %v", err)
			}
			checksum := source.hashFunction.Sum(nil)
			checksumLine := fmt.Sprintf("%s %s\n", hex.EncodeToString(checksum), sourcePath)
			_, err = metadata.Write([]byte(checksumLine))
			if err != nil {
				log.Fatalf("could not write to metadata file: %v", err)
			}
		}

		_, err = w.Add(metadataFile)
		if err != nil {
			log.Fatalf("could not add metadata file: %v", err)
		}

		lastFilesToAdd := []string{".gitignore", "SPECS"}
		for _, f := range lastFilesToAdd {
			_, err := w.Add(f)
			if err != nil {
				log.Fatalf("could not add metadata file: %v", err)
			}
		}

		pd.Importer.PostProcess(md)

		// show status
		status, _ := w.Status()
		log.Printf("successfully processed:\n%s", status)

		var hashes []plumbing.Hash
		var pushRefspecs []config.RefSpec

		head, err := repo.Head()
		if err != nil {
			hashes = nil
			pushRefspecs = append(pushRefspecs, "*:*")
		} else {
			log.Printf("tip %s", head.String())
			hashes = append(hashes, head.Hash())
			refOrigin := "refs/heads/" + md.pushBranch
			pushRefspecs = append(pushRefspecs, config.RefSpec(fmt.Sprintf("HEAD:%s", refOrigin)))
		}

		// we are now finished with the tree and are going to push it to the src repo
		// create import commit
		commit, err := w.Commit("import "+pd.Importer.ImportName(pd, md), &git.CommitOptions{
			Author: &object.Signature{
				Name:  pd.GitCommitterName,
				Email: pd.GitCommitterEmail,
				When:  time.Now(),
			},
			Parents: hashes,
		})
		if err != nil {
			log.Fatalf("could not commit object: %v", err)
		}

		obj, err := repo.CommitObject(commit)
		if err != nil {
			log.Fatalf("could not get commit object: %v", err)
		}

		log.Printf("committed:\n%s", obj.String())

		newTag := "imports/rocky" + strings.TrimPrefix(match[1], "imports/c")
		_, err = repo.CreateTag(newTag, commit, &git.CreateTagOptions{
			Tagger: &object.Signature{
				Name:  pd.GitCommitterName,
				Email: pd.GitCommitterEmail,
				When:  time.Now(),
			},
			Message: "import " + md.tagBranch + " from " + pd.RpmLocation,
			SignKey: nil,
		})
		if err != nil {
			log.Fatalf("could not create tag: %v", err)
		}

		pushRefspecs = append(pushRefspecs, config.RefSpec(fmt.Sprintf("HEAD:%s", plumbing.NewTagReferenceName(newTag))))

		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       pd.Authenticator,
			RefSpecs:   pushRefspecs,
			Force:      true,
		})
		if err != nil {
			log.Fatalf("could not push to remote: %v", err)
		}
	}
}
