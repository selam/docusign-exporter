// Copyright (C) 2023 Timu Eren
//
// This file is part of docusign-exporter.
//
// docusign-exporter is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// docusign-exporter is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with docusign-exporter.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"context"
	"docusign-exporter/config"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
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

func download(envs *model.EnvelopesInformation, wg *sync.WaitGroup, downloaderChan chan<- *model.Envelope) {

	for idx := range envs.Envelopes {
		downloaderChan <- &envs.Envelopes[idx]
		wg.Add(1)
	}

}

func copy(dir string, fname string, dn io.Reader) {
	f, err := os.Create(path.Join(dir, fname))
	if err != nil {
		fmt.Printf("file create: %v\n", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, dn); err != nil {
		fmt.Printf("file create: %v\n", err)
	}
}

func downloaderRoutine(sv *envelopes.Service, downloaderCh <-chan *model.Envelope) {
	for {
		env := <-downloaderCh
		fmt.Println("???")
		dir := path.Join([]string{
			appCfg.App.DownloadFolder,
			string([]rune(env.EnvelopeID)[0]),
			string([]rune(env.EnvelopeID)[0:2]),
			string([]rune(env.EnvelopeID)[0:3]),
		}...)

		err := os.MkdirAll(dir, 0755)
		if err != nil {
			log.Println(err.Error())
		}
		ed, err := json.Marshal(env)
		if err != nil {
			log.Println(err.Error())
		}
		copy(dir, "envelope.json", bytes.NewReader(ed))

		dn, err := sv.DocumentsGet("combined", env.EnvelopeID).
			Certificate().
			Watermark().
			Do(context.TODO())

		if err != nil {
			log.Println(err.Error())
			continue
		}
		copy(dir, "envelope.pdf", dn)
		dn.Close()

	}
}

func checkFolder(f string) {
	stat, err := os.Stat(f)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("download_folder give in configuration file not exists")
		}
		fmt.Println("")
		fmt.Println(err.Error())
		os.Exit(0)
	}
	if !stat.IsDir() {
		fmt.Println("")
		fmt.Println("specified download_folder is not a directory")
		os.Exit(0)
	}

}

func main() {

	p := flag.String("config", "config.json", "path for configuration file")

	flag.CommandLine.Usage = func() {
		flag.PrintDefaults()
		fmt.Println("")
		fmt.Println(`
configuration file is a json file and looks like that

{
	"oauth": {
			"integrator_key":    "XXXXXXXX-xxx-xxx-xxx-XXXXXXXX",
			"secret":           "XXXXXX-xxx-xxx-xxx-XXXXXXXX",
			"redir_url":         "http://localhost/",
			"account_id":        "XXXXXXXX-xxxx-xxxx-xxxx-XXXXXXXX",
			"extended_lifetime": true,
			"is_demo":           true
	},
	"http": {
		"port": 80,
		"host": "0.0.0.0"
	},
	"application": {
		"download_folder": "/tmp",
		"downloader": 1
	}
}

`)
		fmt.Println("")
		fmt.Println("To obtain oauth information from docusign please follow:")
		fmt.Println(">> https://developers.docusign.com/platform/auth/authcode/authcode-get-token/")
		fmt.Println("")
		fmt.Println("")

		os.Exit(0)
	}

	flag.Parse()
	config.Parse(*p, appCfg)

	checkFolder(appCfg.App.DownloadFolder)

	// use gin to obtain and use redirection of token
	codeCh := make(chan string, 1)
	wg := &sync.WaitGroup{}

	downloaderCh := make(chan *model.Envelope, func() int {
		if appCfg.App.DownloadderCount <= 1 {
			return 1
		}
		return appCfg.App.DownloadderCount
	}())

	// gin-gonic settings to obtain "code" value of auth.
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		fmt.Println(c.Query("code"))
		codeCh <- c.Query("code")
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", appCfg.Http.Port),
		Handler: r,
	}

	go func() {
		// // run the web server // gin
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	// just wait and see
	<-time.After(time.Second * 5)
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

	// after obtain to code value, we dont need to run http server anymore,
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("Server Shutdown: %+v\n", err)
	}

	credential, err := cfg.Exchange(ctx, code)
	if err != nil {
		log.Fatal(err)
	}
	sv := envelopes.New(credential)

	for i := 0; i < appCfg.App.DownloadderCount; i++ {
		go downloaderRoutine(sv, downloaderCh)
	}

	from_date, err := time.Parse("2006-02-01", "1970-01-01")
	if err != nil {
		log.Fatal(err)
	}

	envs, err := sv.ListStatusChanges().FromDate(from_date).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// download first files
	download(envs, wg, downloaderCh)
	wg.Wait()
	for {
		if envs.ContinuationToken == "" {
			break
		}
		envs, err := sv.ListStatusChanges().ContinuationToken(envs.ContinuationToken).Do(ctx)
		if err != nil {
			log.Fatal(err)
		}
		download(envs, wg, downloaderCh)
	}

	wg.Wait()
	fmt.Println("All files are downloaded")
}
