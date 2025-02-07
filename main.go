package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
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
	Title string `xml:"title" json:"title"`
	Link  string `xml:"link"  json:"link"`
}

func main() {
	// URL of the RSS feed
	feedURLs := []string{
		"https://news.google.com/rss",
		"https://news.google.com/rss/topics/CAAqJggKIiBDQkFTRWdvSUwyMHZNRGx1YlY4U0FtVnVHZ0pWVXlnQVAB?hl=en-US&gl=US&ceid=US%3Aen", // World
		"https://news.google.com/rss/topics/CAAqJggKIiBDQkFTRWdvSUwyMHZNRGx6TVdZU0FtVnVHZ0pWVXlnQVAB?hl=en-US&gl=US&ceid=US%3Aen", // Business
	}

	var headlines, filteredHeadlines []Item
	/*
		topics := []string{
			"CAAqJggKIiBDQkFTRWdvSUwyMHZNRGx1YlY4U0FtVnVHZ0pWVXlnQVAB", // World
			"CAAqJggKIiBDQkFTRWdvSUwyMHZNRGx6TVdZU0FtVnVHZ0pWVXlnQVAB", // Business
		}
	*/

	f, err := os.Create(filepath.Join("docs", "headlines.json"))
	if err != nil {
		fmt.Println("Error creating or opening the file:", err)
		return
	}
	defer f.Close()

	var prompt strings.Builder
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		log.Fatalf("Error generating client. %v", err)

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
	/*
		model.ResponseSchema = &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"headlines": &genai.Schema{
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"title": &genai.Schema{
								Type: genai.TypeString,
							},
							"link": &genai.Schema{
								Type: genai.TypeString,
							},
						},
					},
				},
			},
		}
	*/

	for _, u := range feedURLs {
		// Fetch RSS feed
		//u := fmt.Sprintf("https://news.google.com/rss/topics/%v/?hl=en-US&gl=US&ceid=US%3Aen", t)
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
		filename := fmt.Sprintf("%v.json", strings.ReplaceAll(rss.Channel.Title, " ", ""))
		f, err := os.Create(filepath.Join("docs", filename))
		if err != nil {
			fmt.Println("Error creating or opening the file:", err)
			return
		}
		defer f.Close()

		// Print feed title
		//fmt.Println("Feed Title:", rss.Channel.Title)

		//prompt.WriteString(fmt.Sprintf("Feed Title: %v\n", rss.Channel.Title))

		// Print the titles and links of the latest articles
		for _, item := range rss.Channel.Items {
			fmt.Printf("- %s (%s)\n", item.Title, item.Link)
			//fmt.Printf("%s\n", item.Title)
			headlines = append(headlines, item)
		}
	}

	for idx, item := range headlines {
		prompt.WriteString(fmt.Sprintf("%d %v\n", idx, item.Title))
	}

	fmt.Println("------------Calling Gemini--------------")
	fmt.Printf("Prompt:\n%s", prompt.String())
	fmt.Println("---------------------")
	chat := model.StartChat()
	res, err := chat.SendMessage(ctx, genai.Text(prompt.String()))
	if err != nil {
		log.Fatalf("Error sending message: %v", err)
	}

	for _, part := range res.Candidates[0].Content.Parts {
		//fmt.Printf("[%d]:\n%v\n", i, part)
		partStr := fmt.Sprintf("%v", part)
		fmt.Printf("partStr = %v\n", partStr)
		ss := strings.Split(partStr, ",")
		for j := 0; j < len(ss); j++ {
			x, err := strconv.Atoi(strings.TrimSpace(ss[j]))
			if err != nil {
				fmt.Printf("Unable to convert to int: %v", err)
				continue
			}
			filteredHeadlines = append(filteredHeadlines, headlines[x])
		}
	}

	jsonData, err := json.MarshalIndent(filteredHeadlines, " ", "    ")
	if err != nil {
		panic(err)
	}
	fmt.Println("Writing to headlines.json")
	if _, err := f.WriteString(string(jsonData)); err != nil {
		panic(err)
	}
}
