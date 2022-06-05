package main

import (
	"fmt"

	"github.com/dunstorm/pm2-go/utils"
)

func main() {
	_, running := utils.IsProcessRunning(30887)
	fmt.Println(running)
}
