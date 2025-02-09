# gnewsrss

## Overview

This is a basic tool to filter political news from RSS feeds using an Large
Language Model (currently Google's Gemini) and outputs the results as a JSON
to a file that can be loaded by a web page.

## Motivation

[Google News](https://news.google.com) doesn't give you much control over what
you don't want to see.  You might be a Baltimore Ravens fan and don't want to
see news about the Pittsburgh Steelers, but Google News doesn't let you
filter out stories involving the Steelers.  This project was motiviated by
wanting to filter out political news stories from Google News.

## How It Works

- Give the tool a file containing the RSS feed URLs.
- The tool will write an array of items to file in JSON format.
- The index.html page will read the JSON file and display the headlines and
  links.

Hosting index.html and the filtered headlines file will make it access.  Since
it is all static, any static page hosting site will work.
