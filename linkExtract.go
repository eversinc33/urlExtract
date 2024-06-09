package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"crypto/tls"

	"github.com/PuerkitoBio/goquery"
	strip "github.com/grokify/html-strip-tags-go"
)

var resultCh = make(chan string)
var wg sync.WaitGroup
var (
	mu         = &sync.Mutex{}
	uniqueUrls = []string{}
)
var recursionStarted = false
var host_url string
var url_no_prefix string
var logOutOfScope *bool
var maxRecursionDepth *int
var cookieString *string

func parseRelativeUrl(u string) string {
	if strings.HasPrefix(u, "http") || strings.HasPrefix(u, "./") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return host_url + u
	}
	// other hrefs refer to the base url
	return host_url + "/" + u
}

func parseLinks(client *http.Client, target_url string, curRecursionDepth int) {
	findings := []string{}

	_, err := url.ParseRequestURI(target_url)
	if err != nil {
		return
	}

	request, err := http.NewRequest("GET", target_url, nil)
	if err != nil {
		log.Fatal(err)
	}
	request.Header.Set("User-Agent", "linkExtract")

	// Make request
	response, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	// Create a goquery document from the HTTP response
	document, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Parse html
	document.Find("a").Each(func(i int, elem *goquery.Selection) {
		href, exists := elem.Attr("href")
		if exists && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "mailto:") && !strings.HasPrefix(href, "xmpp:") && !strings.HasPrefix(href, "javascript") {
			findings = append(findings, parseRelativeUrl(href))
		}
	})
	document.Find("script").Each(func(i int, elem *goquery.Selection) {
		src, exists := elem.Attr("src")
		if exists {
			findings = append(findings, parseRelativeUrl(src))
		}
	})

	// extract from text
	// TODO regex can be improved
	body, _ := io.ReadAll(response.Body)
	bodyStr := string(body)
	r, _ := regexp.Compile(`/[^\s].*`)
	matches := r.FindAllString(bodyStr, -1)
	for _, match := range matches {
		noHtmlTags := strip.StripTags(match)
		findings = append(findings, noHtmlTags)
	}
	r, _ = regexp.Compile(`(http:/|https:/)?(/[^\s"'<>]+)+/?`)
	matches = r.FindAllString(bodyStr, -1)
	for _, match := range matches {
		noHtmlTags := strip.StripTags(match)
		findings = append(findings, noHtmlTags)
	}

	// translate all findings to absolute urls and clean
	for _, foundUrl := range findings {
		result := ""
		if strings.HasPrefix(foundUrl, "http") {
			// absolute url
			result = foundUrl
		} else {
			// relative url
			if strings.HasPrefix(foundUrl, "/") {
				result = target_url + foundUrl
			} else if strings.HasPrefix(foundUrl, "./") {
				result = target_url + trimLeftChars(foundUrl, 2)
			} else {
				result = target_url + "/" + foundUrl
			}
		}

		// start recursive goroutine for each url that was not yet crawled
		if !contains(uniqueUrls, result) {
			if !strings.Contains(result, host_url) && !*logOutOfScope {
				continue
			}

			mu.Lock()
			uniqueUrls = append(uniqueUrls, result)
			mu.Unlock()

			if curRecursionDepth < *maxRecursionDepth {
				wg.Add(1)
				go parseLinks(client, result, curRecursionDepth+1)
			}

			resultCh <- result
		}
	}

	recursionStarted = true // TODO fix this hack
	wg.Done()
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func trimLeftChars(s string, n int) string {
	m := 0
	for i := range s {
		if m >= n {
			return s[i:]
		}
		m++
	}
	return s[:0]
}

func createCookieJarFromString(s string) http.CookieJar {
	header := http.Header{}
	header.Add("Cookie", s)
	request := http.Request{Header: header}

	jar, _ := cookiejar.New(nil)

	if s != "" {
		urlObj, _ := url.Parse(host_url)
		jar.SetCookies(urlObj, request.Cookies())
	}

	return jar
}

func parseArgs() {
	flag.Usage = func() {
		fmt.Println("Usage: linkExtract (flags) [target_url]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	logOutOfScope = flag.Bool("s", false, "Log urls that are not based on the target url and thus out of scope.")
	maxRecursionDepth = flag.Int("r", 1, "Maximum recursion depth.")
	cookieString = flag.String("b", "", "Define cookies to be sent with each request using a string like \"ID=1ymu32x7;SESSION=29\".")

	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	parseArgs()

	// parse url
	target_url := flag.Arg(0)
	u, err := url.ParseRequestURI(target_url)
	if err != nil {
		fmt.Println("Invalid URL. Required format: http(s)://<target>")
		os.Exit(1)
	}
	url_no_prefix = u.Host + u.Path
	host_url = u.Scheme + "://" + u.Host

	tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
        }
        // setup httpclient
        client := &http.Client{
                Timeout: 30 * time.Second,
                Jar:     createCookieJarFromString(*cookieString),
                Transport: tr,
        }

	// crawl
	wg.Add(1)
	go parseLinks(client, target_url, 0)

	go func() {
		for {
			// TODO fix this hack
			if recursionStarted {
				wg.Wait()
				close(resultCh)
				return
			}
		}
	}()

	for result := range resultCh {
		if *logOutOfScope || strings.Contains(result, host_url) {
			fmt.Println(result)
		}
	}
}
