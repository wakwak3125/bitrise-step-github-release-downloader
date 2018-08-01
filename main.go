package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/bitrise-io/go-utils/log"

	"github.com/bitrise-tools/go-steputils/stepconf"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// Config is config
type Config struct {
	AccessToken   stepconf.Secret `env:"github_access_token,required"`
	Tag           string          `env:"tag,required"`
	FileNames     string          `env:"source,required"`
	RepositoryURL string          `env:"repository_url,required"`
}

// Meta is meta
type Meta struct {
	FileName string
	FileURL  string
}

func failf(format string, args ...interface{}) {
	log.Errorf(format, args...)
	os.Exit(1)
}

// formats:
// https://hostname/owner/repository.git
// git@hostname:owner/repository.git
// ssh://git@hostname:port/owner/repository.git
func parseRepo(url string) (host string, owner string, name string) {
	url = strings.TrimSuffix(url, ".git")

	var repo string
	switch {
	case strings.HasPrefix(url, "https://"):
		url = strings.TrimPrefix(url, "https://")
		idx := strings.Index(url, "/")
		host, repo = url[:idx], url[idx+1:]
	case strings.HasPrefix(url, "git@"):
		url = url[strings.Index(url, "@")+1:]
		idx := strings.Index(url, ":")
		host, repo = url[:idx], url[idx+1:]
	case strings.HasPrefix(url, "ssh://"):
		url = url[strings.Index(url, "@")+1:]
		host = url[:strings.Index(url, ":")]
		repo = url[strings.Index(url, "/")+1:]
	}

	split := strings.Split(repo, "/")
	return host, split[0], split[1]
}

func contains(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

func getRelease(
	ctx context.Context,
	client *github.Client,
	tag string,
	owner string,
	repo string) (*github.RepositoryRelease, *github.Response, error) {
	if tag == "latest" {
		return client.Repositories.GetLatestRelease(ctx, owner, repo)
	}
	return client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
}

func main() {
	var c Config
	if err := stepconf.Parse(&c); err != nil {
		failf("Issue with input: %v", err)
	}
	stepconf.Print(c)
	fileNames := strings.Split(c.FileNames, ",")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(c.AccessToken)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	_, owner, repo := parseRepo(c.RepositoryURL)
	release, _, err := getRelease(ctx, client, c.Tag, owner, repo)
	if err != nil {
		failf("Failed to fetch release: %v", err)
	}
	var metas []Meta
	assets := release.Assets
	for _, asset := range assets {
		if contains(fileNames, *asset.Name) {
			metas = append(metas, Meta{asset.GetName(), asset.GetURL()})
		}
	}
	if len(metas) < 1 {
		failf("No such files on target release: %v", err)
	}
	wg := &sync.WaitGroup{}
	for _, meta := range metas {
		wg.Add(1)
		go func(meta Meta) {
			if req, err := http.NewRequest("GET", meta.FileURL, nil); err != nil {
				failf("Failed to create request: %v", err)
			} else {
				req.Header.Add("Authorization", fmt.Sprintf("token %v", string(c.AccessToken)))
				req.Header.Add("Accept", "application/octet-stream")
				log.Infof("Downloading %v", meta.FileName)
				dlClient := new(http.Client)
				res, err := dlClient.Do(req)
				if err != nil {
					failf("Failed to download file: %v", err)
				}
				out, err := os.Create(meta.FileName)
				if err != nil {
					failf("Failed to create file: %v", err)
				}
				defer res.Body.Close()
				_, err = io.Copy(out, res.Body)
				if err != nil {
					failf("Failed to create file: %v", err)
				}
			}
			wg.Done()
		}(meta)
	}
	wg.Wait()
	log.Successf("Success to download files!")
	os.Exit(0)
}
