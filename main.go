package main

import (
	"context"
	"docusign-exporter/config"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jfcote87/esign/v2.1/envelopes"
	"github.com/jfcote87/esign/v2.1/model"
)

var (
	appCfg = &config.Model{}
)

func main() {
	p := flag.String("config", "config.json", "path for configuration file")

	flag.Parse()

	config.Parse(*p, appCfg)

	// use config file
	// to get secrets related with esign.Oauth2Config

	// use gin to obtain and use redirection of token
	r := gin.Default()
	codeCh := make(chan string, 1)
	downloaderCh := make(chan string, func() int {
		if appCfg.App.DownloadderCount <= 1 {
			return 1
		}
		return appCfg.App.DownloadderCount
	}())
	r.GET("/", func(c *gin.Context) {
		fmt.Println(c.Query("code"))
		codeCh <- c.Query("code")
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})
	go func() {
		r.Run(fmt.Sprintf("0.0.0.0:%d", appCfg.Http.Port))
	}()
	<-time.After(time.Second * 10)
	ctx := context.TODO()
	cfg := appCfg.Oauth

	scopes := []string{"signature", "impersonation"}
	authURL := cfg.AuthURL("something", scopes...)
	// Redirect user to consent page.
	fmt.Println()
	fmt.Println()
	fmt.Printf("Visit %s\n", authURL)

	code := <-codeCh
	if code == "" {
		log.Fatalf("code is empty")
	}
	credential, err := cfg.Exchange(ctx, code)
	if err != nil {
		log.Fatal(err)
	}
	sv := envelopes.New(credential)

	for i := 0; i < appCfg.App.DownloadderCount; i++ {
		go func(sv *envelopes.Service) {
			for {
				envId := <-downloaderCh
				dir := path.Join([]string{
					appCfg.App.DownloadFolder,
					string([]rune(envId)[0]),
					string([]rune(envId)[0:1]),
					string([]rune(envId)[0:2]),
				}...)

				err := os.MkdirAll(dir, 0755)
				if err != nil {
					log.Println(err.Error())
				}

				dn, err := sv.DocumentsGet("combined", envId).
					Certificate().
					Watermark().
					Do(context.TODO())

				if err != nil {
					log.Println(err.Error())
					continue
				}

				f, err := os.Create(path.Join(dir, "ds_env.pdf"))
				if err != nil {
					fmt.Printf("file create: %v\n", err)
				}

				if _, err := io.Copy(f, dn); err != nil {
					fmt.Printf("file create: %v\n", err)
				}
				dn.Close()
				f.Close()
			}

		}(sv)
	}

	from_date, err := time.Parse("2006-02-01", "1970-01-01")
	if err != nil {
		log.Fatal(err)
	}
	envs, err := sv.ListStatusChanges().FromDate(from_date).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}
	wg := &sync.WaitGroup{}
	// download first files
	download(envs, wg, downloaderCh)
	for {
		if envs.ContinuationToken == "" {
			break
		}
		envs, err = sv.ListStatusChanges().ContinuationToken(envs.ContinuationToken).Do(ctx)
		if err != nil {
			log.Fatal()
		}
		download(envs, wg, downloaderCh)
	}
	wg.Wait()
	fmt.Println("All files are downloaded")
}

func download(envs *model.EnvelopesInformation, wg *sync.WaitGroup, downloaderChan chan<- string) {
	for idx := range envs.Envelopes {
		downloaderChan <- envs.Envelopes[idx].EnvelopeID
		wg.Add(1)
	}
}
