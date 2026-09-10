package main

import (
	"context"
	"fmt"
	"os"

	"github.com/PuerkitoBio/goquery"
	"github.com/yafyx/baak-api/config"
	"github.com/yafyx/baak-api/utils"
)

func main() {
	config.LoadConfig()
	fmt.Println("Testing session establishment...")
	scraper, err := utils.NewScraper(utils.BaseURL)
	if err != nil {
		fmt.Printf("Error creating scraper: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Session established successfully!")
	fmt.Println("Testing document fetch...")

	// Try different paths to find a working one
	urls := []string{
		utils.BaseURL,
		utils.BaseURL + "/",
		utils.BaseURL + "/jadwal",
		utils.BaseURL + "/jadwal/",
		utils.BaseURL + "/index.php",
		utils.BaseURL + "/index.html",
		utils.BaseURL + "/kalender",
	}

	var doc *goquery.Document
	var successURL string

	for _, testURL := range urls {
		fmt.Printf("Trying URL: %s\n", testURL)
		tempDoc, err := scraper.FetchDocument(context.Background(), testURL)
		if err == nil {
			doc = tempDoc
			successURL = testURL
			break
		}
		fmt.Printf("  Failed: %v\n", err)
	}

	if doc == nil {
		fmt.Println("All URLs failed")
		os.Exit(1)
	}

	fmt.Printf("Document fetched successfully from: %s\n", successURL)
	title := doc.Find("title").Text()
	fmt.Printf("Page title: %s\n", title)
}
