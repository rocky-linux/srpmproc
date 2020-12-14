package internal

import (
    "bytes"
    "crypto/sha1"
    "encoding/hex"
    "fmt"
    "github.com/bluekeyes/go-gitdiff/gitdiff"
    "github.com/cavaliercoder/go-cpio"
    "github.com/cavaliercoder/go-rpm"
    "github.com/go-git/go-billy/v5/memfs"
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/config"
    "github.com/go-git/go-git/v5/plumbing"
    "github.com/go-git/go-git/v5/plumbing/object"
    "github.com/go-git/go-git/v5/plumbing/transport/ssh"
    "github.com/go-git/go-git/v5/storage/memory"
    "io"
    "io/ioutil"
    "log"
    "os"
    "os/exec"
    "os/user"
    "path/filepath"
    "strconv"
    "strings"
    "time"
)

type ProcessData struct {
    RpmLocation    string
    UpstreamPrefix string
    SshKeyLocation string
    SshUser        string
    Branch         string
}

type specFileData struct {
    SourcesToIgnore []string
}

func strContains(a []string, b string) bool {
    for _, val := range a {
        if val == b {
            return true
        }
    }

    return false
}

func parseSpecFile(f []byte, orderedSourceFiles []string) *specFileData {
    spec := specFileData{
        SourcesToIgnore: []string{},
    }

    for _, line := range strings.Split(string(f), "\n") {
        // no need to run through all else ifs
        if strings.HasPrefix(line, "%changelog") {
            break
        }
        orderedSrcLen := len(orderedSourceFiles)

        if strings.HasPrefix(line, "Source") {
            source := strings.SplitN(strings.TrimSpace(line), ":", 2)
            sourceUri := source[1]
            sourceI := strings.Replace(strings.TrimSpace(source[0]), "Source", "", 1)
            i, err := strconv.Atoi(sourceI)
            if err != nil {
                log.Fatalf("could not convert source number: %v", err)
            }

            // if sourceUri contains :// then it's remote
            // so ignore this file
            if strings.Contains(sourceUri, "://") && strings.Contains(sourceUri, ".tar") {
                spec.SourcesToIgnore = append(spec.SourcesToIgnore, orderedSourceFiles[orderedSrcLen-1-i])
            }
        }
    }

    return &spec
}

// ProcessRPM checks the RPM specs and discards any remote files
// This functions also sorts files into directories
// .spec files goes into -> SPECS
// metadata files goes to root
// source files goes into -> SOURCES
// all files that are remote goes into .gitignore
// all ignored files' hash goes into .{name}.metadata
func ProcessRPM(pd *ProcessData) {
    cmd := exec.Command("rpm2cpio", pd.RpmLocation)
    cpioBytes, err := cmd.Output()
    if err != nil {
        log.Fatalf("could not convert to cpio (maybe rpm2cpio is missing): %v", err)
    }

    // create in memory git repository
    repo, err := git.Init(memory.NewStorage(), memfs.New())
    if err != nil {
        log.Fatalf("could not init git repo: %v", err)
    }

    // this will be set later
    specFileName := ""
    var specBts []byte

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

        // set spec file name
        if filepath.Ext(hdr.Name) == ".spec" {
            specFileName = hdr.Name
            var err error
            specBts, err = ioutil.ReadAll(r)
            if err != nil {
                log.Fatalf("could not read spec: %v", err)
            }
            continue
        }

        bts, err := ioutil.ReadAll(r)
        if err != nil {
            log.Fatalf("could not copy file to virtual filesystem: %v", err)
        }
        fileWrites[hdr.Name] = bts
    }

    w, err := repo.Worktree()
    if err != nil {
        log.Fatalf("could not get worktree: %v", err)
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

    // read spec and remove remote files
    spec := parseSpecFile(specBts, rpmFile.Source())
    specFile, err := w.Filesystem.Create(filepath.Join("SPECS", specFileName))
    if err != nil {
        log.Fatalf("could not create spec file: %v", err)
    }
    _, err = specFile.Write(specBts)
    if err != nil {
        log.Fatalf("could not write spec file: %v", err)
    }

    // create a new remote
    remoteUrl := fmt.Sprintf("%s/dist/%s.git", pd.UpstreamPrefix, rpmFile.Name())
    log.Printf("using remote: %s", remoteUrl)
    refspec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", pd.Branch, pd.Branch))
    log.Printf("using refspec: %s", refspec)

    _, err = repo.CreateRemote(&config.RemoteConfig{
        Name:  "origin",
        URLs:  []string{remoteUrl},
        Fetch: []config.RefSpec{refspec},
    })
    if err != nil {
        log.Fatalf("could not create remote: %v", err)
    }

    lastKeyLocation := pd.SshKeyLocation
    if lastKeyLocation == "" {
        usr, err := user.Current()
        if err != nil {
            log.Fatalf("could not get user: %v", err)
        }
        lastKeyLocation = filepath.Join(usr.HomeDir, ".ssh/id_rsa")
    }
    // create ssh key authenticator
    authenticator, err := ssh.NewPublicKeysFromFile(pd.SshUser, lastKeyLocation, "")
    if err != nil {
        log.Fatalf("could not get git authenticator: %v", err)
    }
    err = repo.Fetch(&git.FetchOptions{
        RemoteName: "origin",
        RefSpecs:   []config.RefSpec{refspec},
        Auth:       authenticator,
    })

    refName := plumbing.NewBranchReferenceName(pd.Branch)
    log.Printf("set reference to ref: %s", refName)

    if err != nil {
        h := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
        if err := repo.Storer.CheckAndSetReference(h, nil); err != nil {
            log.Fatalf("could not set reference: %v", err)
        }
    } else {
        err = w.Checkout(&git.CheckoutOptions{
            Branch: plumbing.NewRemoteReferenceName("origin", pd.Branch),
            Force:  true,
        })
        if err != nil {
            log.Fatalf("could not checkout: %v", err)
        }
    }

    // TODO: Remove dangling files

    for fileName, contents := range fileWrites {
        newPath := filepath.Join("SOURCES", fileName)

        // add the file to the virtual filesystem
        // we will move it to correct destination later
        f, err := w.Filesystem.Create(newPath)
        if err != nil {
            log.Fatalf("could not create file %s: %v", fileName, err)
        }

        _, err = f.Write(contents)
        if err != nil {
            log.Fatalf("could not write to file %s: %v", fileName, err)
        }

        _ = f.Close()

        // don't add ignored file to git
        if strContains(spec.SourcesToIgnore, fileName) {
            continue
        }

        log.Printf("git add %s", newPath)

        _, err = w.Add(newPath)
        if err != nil {
            log.Fatalf("could not add source file: %v", err)
        }
    }

    // add sources to ignore (remote sources)
    gitIgnore, err := w.Filesystem.Create(".gitignore")
    if err != nil {
        log.Fatalf("could not create .gitignore: %v", err)
    }
    for _, ignore := range spec.SourcesToIgnore {
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

    // check if patches exist
    _, err = w.Filesystem.Stat("ROCKY")
    if err == nil {
        // check SRPM patches
        _, err = w.Filesystem.Stat("ROCKY/SRPM")
        if err == nil {
            // iterate through patches
            infos, err := w.Filesystem.ReadDir("ROCKY/SRPM")
            if err != nil {
                log.Fatalf("could not walk patches: %v", err)
            }

            for _, info := range infos {
                // can only process .patch files
                if !strings.HasSuffix(info.Name(), ".patch") {
                    continue
                }

                log.Printf("applying %s", info.Name())
                filePath := filepath.Join("ROCKY/SRPM", info.Name())

                patch, err := w.Filesystem.Open(filePath)
                if err != nil {
                    log.Fatalf("could not open patch file %s: %v", info.Name(), err)
                }
                files, _, err := gitdiff.Parse(patch)
                if err != nil {
                    log.Fatalf("could not parse patch file: %v", err)
                }

                for _, patchedFile := range files {
                    srcPath := filepath.Join("SOURCES", patchedFile.NewName)
                    patchSubjectFile, err := w.Filesystem.Open(srcPath)
                    if err != nil {
                        log.Fatalf("could not open patch subject: %v", err)
                    }

                    var output bytes.Buffer
                    err = gitdiff.NewApplier(patchSubjectFile).ApplyFile(&output, patchedFile)
                    if err != nil {
                        log.Fatalf("could not apply patch: %v", err)
                    }

                    err = w.Filesystem.Remove(srcPath)
                    if err != nil {
                        log.Fatalf("could not remove pre-patch file: %v", err)
                    }

                    newFile, err := w.Filesystem.Create(srcPath)
                    if err != nil {
                        log.Fatalf("could not create post-patch file: %v", err)
                    }
                    _, err = newFile.Write(output.Bytes())
                    if err != nil {
                        log.Fatalf("could not write post-patch file: %v", err)
                    }
                    _, err = w.Add(srcPath)
                    if err != nil {
                        log.Fatalf("could not add file %s to git: %v", srcPath, err)
                    }
                    log.Printf("git add %s", srcPath)
                }

                _, err = w.Add(filePath)
                if err != nil {
                    log.Fatalf("could not add file %s to git: %v", filePath, err)
                }
                log.Printf("git add %s", filePath)
            }
        }
    }

    // get ignored files sha1 hash and add to .{name}.metadata
    metadataFile := fmt.Sprintf(".%s.metadata", rpmFile.Name())
    metadata, err := w.Filesystem.Create(metadataFile)
    if err != nil {
        log.Fatalf("could not create metadata file: %v", err)
    }
    for _, source := range spec.SourcesToIgnore {
        sourcePath := "SOURCES/" + source
        sourceFile, err := w.Filesystem.Open(sourcePath)
        if err != nil {
            log.Fatalf("could not open ignored source file %s: %v", sourcePath, err)
        }
        sourceFileBts, err := ioutil.ReadAll(sourceFile)
        if err != nil {
            log.Fatalf("could not read the whole of ignored source file: %v", err)
        }

        checksum := sha1.Sum(sourceFileBts)
        checksumLine := fmt.Sprintf("%s %s\n", hex.EncodeToString(checksum[:]), sourcePath)
        _, err = metadata.Write([]byte(checksumLine))
        if err != nil {
            log.Fatalf("could not write to metadata file: %v", err)
        }
    }

    lastFilesToAdd := []string{metadataFile, ".gitignore", "SPECS"}
    for _, f := range lastFilesToAdd {
        _, err := w.Add(f)
        if err != nil {
            log.Fatalf("could not add metadata file: %v", err)
        }
    }

    // show status
    status, _ := w.Status()
    log.Printf("successfully processed:\n%s", status)

    head, err := repo.Head()
    if err != nil {
        log.Fatalf("could not get ref: %v", err)
    }
    log.Printf("tip %s", head.String())

    // we are now finished with the tree and are going to push it to the src repo
    // create import commit
    commit, err := w.Commit("import "+filepath.Base(pd.RpmLocation), &git.CommitOptions{
        Author: &object.Signature{
            Name:  "Mustafa Gezen",
            Email: "mustafa@bycrates.com",
            When:  time.Now(),
        },
        Parents: []plumbing.Hash{head.Hash()},
    })
    if err != nil {
        log.Fatalf("could not commit object: %v", err)
    }

    obj, err := repo.CommitObject(commit)
    if err != nil {
        log.Fatalf("could not get commit object: %v", err)
    }

    log.Printf("committed:\n%s", obj.String())

    refOrigin := "refs/heads/" + pd.Branch
    err = repo.Push(&git.PushOptions{
        RemoteName: "origin",
        Auth:       authenticator,
        RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("HEAD:%s", refOrigin))},
    })
    if err != nil {
        log.Fatalf("could not push to remote: %v", err)
    }
}
