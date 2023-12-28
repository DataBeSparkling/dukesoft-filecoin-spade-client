package ariaclient

import (
	"context"
	"filecoin-spade-client/pkg/config"
	"filecoin-spade-client/pkg/log"
	"github.com/siku2/arigo"
	"sync"
)

type AriaClient struct {
	Config         config.AriaConfig
	Client         *arigo.Client
	DownloadPath   string
	Downloads      map[string]arigo.Status
	DownloadsMutex sync.Mutex
	Keys           []string
	AriaMutex      sync.Mutex
}

func New(config config.Configuration) *AriaClient {
	ac := new(AriaClient)
	ac.Config = config.AriaConfig
	ac.DownloadPath = config.DownloadPath
	ac.Keys = []string{"gid", "status", "totalLength", "completedLength", "downloadSpeed", "files"}
	return ac
}

func (ac *AriaClient) Start(ctx context.Context) {
	c, err := arigo.Dial(ac.Config.Url, ac.Config.AuthToken)
	if err != nil {
		log.Fatalf("error starting aria2 client: %+v", err)
	}
	ac.Client = &c

	if ac.UpdateDownloads() != nil {
		log.Fatalf("error fetching aria2c downloads: %s", err)
	}

	log.Infof("Successfully connected to Aria2C service")

	go func() {
		select {
		case <-ctx.Done():
			log.Infof("shutting down Aria2C service: context done")

			return
		}
	}()
}

func (ac *AriaClient) UpdateDownloads() error {
	allDownloads := make(map[string]arigo.Status)
	activeDownloads, err := ac.Client.TellActive(ac.Keys...)
	if err != nil {
		return err
	}
	log.Debugf("Total active downloads: %d", len(activeDownloads))
	for _, download := range activeDownloads {
		allDownloads[download.GID] = download
	}

	waitingDownloads, err := ac.Client.TellWaiting(0, 99999, ac.Keys...)
	if err != nil {
		return err
	}
	log.Debugf("Total waiting downloads: %d", len(waitingDownloads))
	for _, download := range waitingDownloads {
		allDownloads[download.GID] = download
	}

	stoppedDownloads, err := ac.Client.TellStopped(0, 99999, ac.Keys...)
	if err != nil {
		return err
	}
	log.Debugf("Total stopped downloads: %d", len(stoppedDownloads))
	for _, download := range stoppedDownloads {
		allDownloads[download.GID] = download
	}

	ac.DownloadsMutex.Lock()
	ac.Downloads = allDownloads
	ac.DownloadsMutex.Unlock()

	return nil
}

func (ac *AriaClient) FindDownloadByUri(searchingUri string) *arigo.Status {
	ac.DownloadsMutex.Lock()
	defer ac.DownloadsMutex.Unlock()
	for _, dl := range ac.Downloads {
		for _, file := range dl.Files {
			for _, uri := range file.URIs {
				if searchingUri == uri.URI {
					return &dl
				}
			}
		}
	}
	return nil
}

func (ac *AriaClient) NewDownload(url string, downloadFilename string) (arigo.GID, error) {
	log.Infof("Starting download [%s] to file [%s]", url, downloadFilename)
	ac.AriaMutex.Lock()
	defer ac.AriaMutex.Unlock()
	return ac.Client.AddURI([]string{url}, &arigo.Options{
		AlwaysResume:   true,
		Continue:       true,
		Dir:            ac.DownloadPath,
		DryRun:         false,
		FileAllocation: "falloc",
		ForceSave:      false,
		Out:            downloadFilename,
	})
}

func (ac *AriaClient) GetStatus(gid string) (arigo.Status, error) {
	ac.AriaMutex.Lock()
	defer ac.AriaMutex.Unlock()
	return ac.Client.TellStatus(gid, ac.Keys...)
}

func (ac *AriaClient) RemoveDownload(gid string) error {
	ac.AriaMutex.Lock()
	defer ac.AriaMutex.Unlock()
	return ac.Client.RemoveDownloadResult(gid)
}
