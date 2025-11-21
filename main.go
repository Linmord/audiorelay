package main

import (
	"audiorelay/audiorelay"
	"fmt"
)

func main() {
	if err := audiorelay.StartWithConfig("config.yml"); err != nil {
		fmt.Println(err)
	}
}
