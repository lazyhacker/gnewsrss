package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/mmcdole/gofeed"
	"google.golang.org/api/option"
)

type Item struct {
	Title string `xml:"title" json:"title"`
	Link  string `xml:"link"  json:"link"`
}

var debug bool

func main() {

	flag.BoolVar(&debug, "debug", false, "Enable debug mode.")
	flag.Parse()

	// Get the list of feeds to fetch.
	feedUrls, err := FeedUrls("feeds.txt")
	if err != nil {
		panic(err)
	}

	// Fetch the items from all the feeds.
	headlines := FetchRSS(feedUrls)

	// Filter out political items from the list.
	filteredHeadlines, discardedHeadlines, err := Filter(headlines)
	if err != nil {
		panic(err)
	}

	if debug {
		log.Println("All Headlines\n")
		for i, v := range headlines {
			log.Printf("%d %v\n", i, v.Title)
		}
		log.Println("\n********Accepted Headlines**********\n")
		for _, v := range filteredHeadlines {
			log.Printf("%v\n", v.Title)
		}
		log.Println("\n**************Dropped Headlines***********\n")
		for _, v := range discardedHeadlines {
			log.Printf("%v\n", v.Title)
		}
	}
	if !debug {
		// Format the filtered list as JSON
		jsonData, err := json.MarshalIndent(filteredHeadlines, " ", "    ")
		if err != nil {
			panic(err)
		}

		// Write the JSON to file.
		log.Print("Writing to headlines.json")
		f, err := os.Create(filepath.Join("docs", "headlines.json"))
		if err != nil {
			log.Print("Error creating or opening the file:", err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(string(jsonData)); err != nil {
			panic(err)
		}
	}
}

// FeedUrls returns the list of RSS feed URLs from the configuration.
func FeedUrls(feedsfile string) ([]string, error) {

	// Open feeds file with the URLs of RSS feeds
	ff, err := os.Open(feedsfile)
	if err != nil {
		return nil, fmt.Errorf("Unable to open feeds file. %v.", err)
	}
	defer ff.Close() // Ensure file is closed

	// Create a scanner to read the file line by line
	var feedURLs []string
	scanner := bufio.NewScanner(ff)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text()) // Trim spaces

		// Ignore blank lines and lines starting with "#"
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		feedURLs = append(feedURLs, line) // Append valid lines
	}

	// Check for errors while reading
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Unable to read the feeds file. %v", err)
	}

	return feedURLs, nil
}

// FetchRSS takes in a list of RSS feeds, fetches the items from the feeds
// and return them in a single list.  It will remove items that are considered
// old.
func FetchRSS(feedURLs []string) []*gofeed.Item {

	var headlines []*gofeed.Item

	fp := gofeed.NewParser()
	fp.UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	published_layout := "Mon, 02 Jan 2006 15:04:05 MST"
	expiryDate := time.Now().AddDate(0, -1, 0) // 1 month ago

	for _, u := range feedURLs {
		// Fetch RSS feed
		/*
			resp, err := http.Get(u)
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
		*/
		rss, err := fp.ParseURL(u)
		if err != nil {
			log.Printf("Error fetching feed %v: %v", u, err)
			continue
		}
		// Print the titles and links of the latest articles
		for _, item := range rss.Items {
			//fmt.Printf("- %s (%s)\n", item.Title, item.Link)
			pubDate, err := time.Parse(published_layout, item.Published)
			if err == nil && pubDate.Before(expiryDate) {
				//log.Println("published date greater then 1 month.  skipping...")
				continue
			}

			if err != nil {
				log.Printf("Unable to parse the published date. %v", err)
			}

			// if unable to parse date OR pubDate is less then one month, add to headline
			headlines = append(headlines, item)
		}
	}

	return headlines
}

func Filter(headlines []*gofeed.Item) ([]*gofeed.Item, []*gofeed.Item, error) {

	var filteredHeadlines, discardedHeadlines []*gofeed.Item
	var prompt strings.Builder

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		return nil, nil, fmt.Errorf("Error generating Gemini client. %v", err)

	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")
	model.SetTemperature(1)
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(8192)
	//model.ResponseMIMEType = "application/json"
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text("You are a news filter who will be given a list of news headlines prepended with an index value to filter out any that are political in nature.  Any headline that mentions Trump are considered political and should be removed.  Also filter out any headlines that include individuals such as Elon Musk who are political figures even though they aren't politicians. Also filter out individuals who are known wealthy politicial donors. Only return the index value of the headlines that are non-political as a comma separated list.")},
	}

	for idx, item := range headlines {
		//fmt.Printf("%d %v\n", idx, item.Title)
		prompt.WriteString(fmt.Sprintf("%d %v\n", idx, item.Title))
	}

	log.Println("------------Calling Gemini--------------")
	//fmt.Printf("Prompt:\n%s", prompt.String())
	chat := model.StartChat()
	res, err := chat.SendMessage(ctx, genai.Text(prompt.String()))
	if err != nil {
		return nil, nil, fmt.Errorf("Error sending message to Gemini. %v", err)
	}

	for _, part := range res.Candidates[0].Content.Parts {
		partStr := fmt.Sprintf("%v", part)
		if debug {
			log.Printf("accepted headlines = %v\n", partStr)
		}
		ss := strings.Split(partStr, ",")
		dropCounter := -1
		for j := 0; j < len(ss); j++ {
			x, err := strconv.Atoi(strings.TrimSpace(ss[j]))
			if err != nil {
				log.Printf("Unable to convert to int: %v", err)
				continue
			}

			if x >= len(headlines) {
				log.Printf("x = %d while len(headlines) = %d\n", x, len(headlines))
				continue
			}
			//fmt.Printf("headlines[%d] = %v\n", x, headlines[x].Title)
			filteredHeadlines = append(filteredHeadlines, headlines[x])

			// The LLM returns the the sequence of headlines indices for headlines
			// to the kept so the gap between the returned numbers are the
			// headlines that were filtered out
			for _, y := range gap(dropCounter, x) {
				discardedHeadlines = append(discardedHeadlines, headlines[y])
			}
			dropCounter = x // remember so can find the gap with the next index
		}

		// Do one more check to see if there is a gap between the last
		// accepted headline to the end of the original headlines.
		for _, y := range gap(dropCounter, len(headlines)-1) {
			discardedHeadlines = append(discardedHeadlines, headlines[y])
		}
	}

	return filteredHeadlines, discardedHeadlines, nil
}

// gap returns the missing numbers between two given integers.
func gap(a, b int) []int {

	var intgap []int

	g := b - a - 1
	if g <= 0 {
		return nil
	}

	for i := a + 1; i <= a+g; i++ {
		intgap = append(intgap, i)
	}
	return intgap
}
