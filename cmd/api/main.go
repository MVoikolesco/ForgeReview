// Command api starts ForgeReview using the mode selected by APP_MODE.
package main

import (
	"log"

	"gitea-agents/internal/app"
)

// main runs ForgeReview and terminates the process on startup or runtime errors.
func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
