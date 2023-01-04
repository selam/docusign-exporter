package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

func Parse(p string, m *Model) {
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		fmt.Println("config file not found, please create a config file",
			"for more information try -h or --help")
		os.Exit(0)
	}

	dt, err := os.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(dt, m); err != nil {
		log.Fatal(err)
	}
}
