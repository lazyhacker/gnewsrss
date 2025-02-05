package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
)

// RSS struct to parse RSS feed

type RSS struct {
	Channel Channel `xml:"channel"`
}

type Channel struct {
	Title string `xml:"title"`
	Items []Item `xml:"item"`
}

type Item struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

func main() {
	// URL of the RSS feed
	feedURL := "https://news.google.com/rss"

	// Fetch RSS feed
	resp, err := http.Get(feedURL)
	if err != nil {
		log.Fatalf("Error fetching RSS feed: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse RSS feed
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading RSS feed: %v", err)
	}

	var rss RSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		log.Fatalf("Error parsing RSS feed: %v", err)
	}

	// Print feed title
	fmt.Println("Feed Title:", rss.Channel.Title)

	// Print the titles and links of the latest articles
	for _, item := range rss.Channel.Items {
		//fmt.Printf("- %s (%s)\n", item.Title, item.Link)
		fmt.Printf("- %s\n", item.Title)
	}
}
