package config

import "github.com/jfcote87/esign"

type Model struct {
	Oauth *esign.OAuth2Config `json:"oauth"`
	Http  struct {
		Port int    `json:"port"`
		Host string `json:"host"`
	} `json:"http"`
	App struct {
		DownloadFolder   string `json:"download_folder"`
		DownloadderCount int    `json:"downloader"`
	} `json:"application"`
}
