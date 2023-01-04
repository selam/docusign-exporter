package config

import (
	"encoding/json"
	"log"
	"os"
)

func Parse(p string, m *Model) {
	dt, err := os.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(dt, m); err != nil {
		log.Fatal(err)
	}
}
