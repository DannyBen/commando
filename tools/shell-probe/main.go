package main

import (
	"log"

	"github.com/dannyben/commando/internal/shellprobe"
)

func main() {
	if err := shellprobe.Run(); err != nil {
		log.Fatal(err)
	}
}
