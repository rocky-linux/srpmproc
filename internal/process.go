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
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var tagImportRegex *regexp.Regexp

type ProcessData struct {
	RpmLocation        string
	UpstreamPrefix     string
	SshKeyLocation     string
	SshUser            string
	Version            int
	GitCommitterName   string
	GitCommitterEmail  string
	Mode               int
	ModulePrefix       string
	ImportBranchPrefix string
	BranchPrefix       string
	Authenticator      *ssh.PublicKeys
	Importer           ImportMode
	BlobStorage        blob.Storage
	NoDupMode          bool
	ModuleMode         bool
}

type ignoredSource struct {
	name         string
	hashFunction hash.Hash
	expired      bool
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
	tagImportRegex = regexp.MustCompile(fmt.Sprintf("refs/tags/(imports/(%s.|%s.-.+)/(.*))", pd.ImportBranchPrefix, pd.ImportBranchPrefix))
	md := pd.Importer.RetrieveSource(pd)

	remotePrefix := "rpms"
	if pd.ModuleMode {
		remotePrefix = "modules"
	}

	// if no-dup-mode is enabled then skip already imported versions
	var tagIgnoreList []string
	if pd.NoDupMode {
		repo, err := git.Init(memory.NewStorage(), memfs.New())
		if err != nil {
			log.Fatalf("could not init git repo: %v", err)
		}
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, md.rpmFile.Name())
		refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")

		remote, err := repo.CreateRemote(&config.RemoteConfig{
			Name:  "origin",
			URLs:  []string{remoteUrl},
			Fetch: []config.RefSpec{refspec},
		})
		if err != nil {
			log.Fatalf("could not create remote: %v", err)
		}

		list, err := remote.List(&git.ListOptions{
			Auth: pd.Authenticator,
		})
		if err != nil {
			log.Println("ignoring no-dup-mode")
		} else {
			for _, ref := range list {
				if !strings.HasPrefix(string(ref.Name()), "refs/tags/imports") {
					continue
				}
				tagIgnoreList = append(tagIgnoreList, string(ref.Name()))
			}
		}
	}

	sourceRepo := *md.repo
	sourceWorktree := *md.worktree

	for _, branch := range md.branches {
		md.repo = &sourceRepo
		md.worktree = &sourceWorktree
		md.tagBranch = branch
		for _, source := range md.sourcesToIgnore {
			source.expired = true
		}

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

		var matchString string
		if !tagImportRegex.MatchString(md.tagBranch) {
			if pd.ModuleMode {
				prefix := fmt.Sprintf("refs/heads/%s%d", pd.ImportBranchPrefix, pd.Version)
				if strings.HasPrefix(md.tagBranch, prefix) {
					replace := strings.Replace(md.tagBranch, "refs/heads/", "", 1)
					matchString = fmt.Sprintf("refs/tags/imports/%s/%s", replace, filepath.Base(pd.RpmLocation))
					log.Printf("using match string: %s", matchString)
				}
			}
			if !tagImportRegex.MatchString(matchString) {
				continue
			}
		} else {
			matchString = md.tagBranch
		}

		match := tagImportRegex.FindStringSubmatch(matchString)
		md.pushBranch = pd.BranchPrefix + strings.TrimPrefix(match[2], "c")
		newTag := "imports/" + pd.BranchPrefix + strings.TrimPrefix(match[1], "imports/c")

		shouldContinue := true
		for _, ignoredTag := range tagIgnoreList {
			if ignoredTag == "refs/tags/"+newTag {
				log.Printf("skipping %s", ignoredTag)
				shouldContinue = false
			}
		}
		if !shouldContinue {
			continue
		}

		// create a new remote
		remoteUrl := fmt.Sprintf("%s/%s/%s.git", pd.UpstreamPrefix, remotePrefix, rpmFile.Name())
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

		if pd.ModuleMode {
			patchModuleYaml(pd, md)
		} else {
			executePatchesRpm(pd, md)
		}

		// already uploaded blobs are skipped
		var alreadyUploadedBlobs []string
		// get ignored files hash and add to .{name}.metadata
		metadataFile := fmt.Sprintf(".%s.metadata", rpmFile.Name())
		metadata, err := w.Filesystem.Create(metadataFile)
		if err != nil {
			log.Fatalf("could not create metadata file: %v", err)
		}
		for _, source := range md.sourcesToIgnore {
			sourcePath := source.name

			_, err := w.Filesystem.Stat(sourcePath)
			if source.expired || err != nil {
				continue
			}

			sourceFile, err := w.Filesystem.Open(sourcePath)
			if err != nil {
				log.Fatalf("could not open ignored source file %s: %v", sourcePath, err)
			}
			sourceFileBts, err := ioutil.ReadAll(sourceFile)
			if err != nil {
				log.Fatalf("could not read the whole of ignored source file: %v", err)
			}

			source.hashFunction.Reset()
			_, err = source.hashFunction.Write(sourceFileBts)
			if err != nil {
				log.Fatalf("could not write bytes to hash function: %v", err)
			}
			checksum := hex.EncodeToString(source.hashFunction.Sum(nil))
			checksumLine := fmt.Sprintf("%s %s\n", checksum, sourcePath)
			_, err = metadata.Write([]byte(checksumLine))
			if err != nil {
				log.Fatalf("could not write to metadata file: %v", err)
			}

			path := checksum
			if strContains(alreadyUploadedBlobs, path) {
				continue
			}
			pd.BlobStorage.Write(path, sourceFileBts)
			log.Printf("wrote %s to blob storage", path)
			alreadyUploadedBlobs = append(alreadyUploadedBlobs, path)
		}

		_, err = w.Add(metadataFile)
		if err != nil {
			log.Fatalf("could not add metadata file: %v", err)
		}

		lastFilesToAdd := []string{".gitignore", "SPECS"}
		for _, f := range lastFilesToAdd {
			_, err := w.Filesystem.Stat(f)
			if err == nil {
				_, err := w.Add(f)
				if err != nil {
					log.Fatalf("could not add %s: %v", f, err)
				}
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
