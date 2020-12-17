package main

import (
	"cloud.google.com/go/storage"
	"context"
	"log"
	"strings"

	"github.com/mstg/srpmproc/internal"
	"github.com/spf13/cobra"
)

var (
	sourceRpm         string
	sshKeyLocation    string
	sshUser           string
	upstreamPrefix    string
	branch            string
	gcsBucket         string
	gitCommitterName  string
	gitCommitterEmail string
)

var root = &cobra.Command{
	Use: "srpmproc",
	Run: mn,
}

func mn(_ *cobra.Command, _ []string) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("could not create gcloud client: %v", err)
	}

	sourceRpmLocation := ""
	if strings.HasPrefix(sourceRpm, "file://") {
		sourceRpmLocation = strings.TrimPrefix(sourceRpm, "file://")
	} else {
		log.Fatal("non-local SRPMs are currently not supported")
	}

	internal.ProcessRPM(&internal.ProcessData{
		RpmLocation:       sourceRpmLocation,
		UpstreamPrefix:    upstreamPrefix,
		SshKeyLocation:    sshKeyLocation,
		SshUser:           sshUser,
		Branch:            branch,
		Bucket:            client.Bucket(gcsBucket),
		GitCommitterName:  gitCommitterName,
		GitCommitterEmail: gitCommitterEmail,
	})
}

func main() {
	root.Flags().StringVar(&sourceRpm, "source-rpm", "", "Location of RPM to process")
	_ = root.MarkFlagRequired("source-rpm")
	root.Flags().StringVar(&upstreamPrefix, "upstream-prefix", "", "Upstream git repository prefix")
	_ = root.MarkFlagRequired("upstream-prefix")
	root.Flags().StringVar(&branch, "branch", "", "Upstream branch")
	_ = root.MarkFlagRequired("branch")
	root.Flags().StringVar(&gcsBucket, "gcs-bucket", "", "Bucket to use as blob storage")
	_ = root.MarkFlagRequired("gcs-bucket")
	root.Flags().StringVar(&gitCommitterName, "git-committer-name", "distrobuild-bot", "Name of committer")
	root.Flags().StringVar(&gitCommitterEmail, "git-committer-email", "mustafa+distrobuild@bycrates.com", "Email of committer")

	root.Flags().StringVar(&sshKeyLocation, "ssh-key-location", "", "Location of the SSH key to use to authenticate against upstream")
	root.Flags().StringVar(&sshUser, "ssh-user", "git", "SSH User")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
