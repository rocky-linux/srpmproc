package main

import (
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
    debrandedTarballs []string
)

var root = &cobra.Command{
    Use: "srpmproc",
    Run: mn,
}

func mn(_ *cobra.Command, _ []string) {
    sourceRpmLocation := ""
    if strings.HasPrefix(sourceRpm, "file://") {
        sourceRpmLocation = strings.TrimPrefix(sourceRpm, "file://")
    } else {
        log.Fatal("non-local SRPMs are currently not supported")
    }

    internal.ProcessRPM(&internal.ProcessData{
        RpmLocation:    sourceRpmLocation,
        UpstreamPrefix: upstreamPrefix,
        SshKeyLocation: sshKeyLocation,
        SshUser:        sshUser,
        Branch:         branch,
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

    root.Flags().StringVar(&sshKeyLocation, "ssh-key-location", "", "Location of the SSH key to use to authenticate against upstream (Optional)")
    root.Flags().StringVar(&sshUser, "ssh-user", "git", "SSH User (Optional, default git)")
    root.Flags().StringArrayVar(&debrandedTarballs, "debranded-tarball", []string{}, "GCS urls to debranded tarballs (stage 2) (Optional)")

    if err := root.Execute(); err != nil {
        log.Fatal(err)
    }
}
