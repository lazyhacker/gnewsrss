package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/mmcdole/gofeed"
	"google.golang.org/api/option"
)

var (
	debug  bool
	model  string
	instrf string
)

func main() {

	flag.BoolVar(&debug, "debug", false, "Enable debug mode.")
	flag.StringVar(&model, "model", "gemini-2.0-flash", "Gemini model")
	feedsConfig := flag.String("feeds", "feeds.txt", "File containing feed URLs to fetch.")
	flag.StringVar(&instrf, "instruction", "instruction.txt", "Path to file containing the filter instructions.")
	outfile := flag.String("out", "", "Output file")
	flag.Parse()

	// Get the list of feeds to fetch.
	feedUrls, err := FeedUrls(*feedsConfig)
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
		fmt.Println("All Headlines\n")
		for i, v := range headlines {
			fmt.Printf("%d %v\n", i, v.Title)
		}
		fmt.Println("\n********Accepted Headlines**********\n")
		for _, v := range filteredHeadlines {
			fmt.Printf("%v\n", v.Title)
		}
		fmt.Println("\n**************Dropped Headlines***********\n")
		for _, v := range discardedHeadlines {
			fmt.Printf("%v\n", v.Title)
		}
	}

	// Format the filtered list as JSON
	jsonData, err := json.MarshalIndent(filteredHeadlines, " ", "    ")
	if err != nil {
		panic(err)
	}

	if len(*outfile) > 0 {
		// Write the JSON to file.
		log.Print("Writing to headlines.json")
		f, err := os.Create(*outfile)
		if err != nil {
			log.Print("Error creating or opening the file:", err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString(string(jsonData)); err != nil {
			panic(err)
		}
	} else {
		fmt.Printf("%v\n", string(jsonData))
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
		rss, err := fp.ParseURL(u)
		if err != nil {
			log.Printf("Error fetching feed %v: %v", u, err)
			continue
		}
		// Print the titles and links of the latest articles
		for _, item := range rss.Items {

			pubDate, err := time.Parse(published_layout, item.Published)

			// Don't keep items that are older then expirayDate old.
			if err == nil && pubDate.Before(expiryDate) {
				continue
			}

			// If unable to parse the date then just keep the item.  Logging
			// just to see how often this happens.
			if err != nil {
				log.Printf("Unable to parse the published date. %v", err)
			}

			headlines = append(headlines, item)
		}
	}
	return headlines
}

// Filter calls Gemini API to have it filter the items.
func Filter(headlines []*gofeed.Item) ([]*gofeed.Item, []*gofeed.Item, error) {

	var filteredHeadlines, discardedHeadlines []*gofeed.Item
	var prompt strings.Builder

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		return nil, nil, fmt.Errorf("Error generating Gemini client. %v", err)

	}
	defer client.Close()

	instruction, err := ioutil.ReadFile(instrf)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to read instruction file. %v", err)
	}
	model := client.GenerativeModel(model)
	model.SetTemperature(1)
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(8192) // More then sufficient when returning just the index.
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(instruction)},
	}

	for idx, item := range headlines {
		prompt.WriteString(fmt.Sprintf("%d %v\n", idx, item.Title))
	}

	log.Println("------------Calling Gemini--------------")
	chat := model.StartChat()
	res, err := chat.SendMessage(ctx, genai.Text(prompt.String()))
	if err != nil {
		return nil, nil, fmt.Errorf("Error sending message to Gemini. %v", err)
	}

	for _, part := range res.Candidates[0].Content.Parts {
		partStr := fmt.Sprintf("%v", part)
		if debug {
			fmt.Printf("accepted headlines = %v\n", partStr)
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
