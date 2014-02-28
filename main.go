package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	NUM_RESOLVERS = 3
)

var (
	errDone = errors.New("not following any more redirects")
	prots   = []string{
		"http",
		"https",
	}
)

type RedirectBehavior func(req *http.Request, via []*http.Request) error

var (
	// Follow all redirects, only find out about final pages.
	FollowAllRedirects = func(req *http.Request, via []*http.Request) error {
		return nil
	}
	// Stop on first redirect encountered.
	StopOnFirstRedirect = func(req *http.Request, via []*http.Request) error {
		return errDone
	}
	// Stop as soon as you are redirected to a different domain.
	StopOnRedirectToDifferentDomain = func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != via[len(via)-1].URL.Host {
			return errDone
		}
		return nil
	}
)

type Site struct {
	RequestUrl  url.URL
	StatusCode  int
	ResponseUrl *url.URL
}

func (s *Site) String() string {
	var line = fmt.Sprintf("%s,%d,", s.RequestUrl.Host, s.StatusCode)
	if s.ResponseUrl != nil {
		line += s.ResponseUrl.String()
	}
	return line
}

func main() {
	// start result writer
	var writerGroup sync.WaitGroup
	var result = make(chan *Site)
	writerGroup.Add(1)
	go writeWorker(&writerGroup, result)
	// start url reader
	var work = make(chan string)
	go readWorker("urls.txt", work)
	// start checkers
	var checkRedirect = FollowAllRedirects
	var workerGroup sync.WaitGroup
	for i := 0; i < NUM_RESOLVERS; i++ {
		workerGroup.Add(1)
		go checkWorker(&workerGroup, work, result, checkRedirect)
	}
	// wait for all checkers to finish
	workerGroup.Wait()
	close(result)
	// wait for writer to finish
	writerGroup.Wait()
}

func readWorker(filepath string, work chan<- string) {
	defer close(work)
	reader := openUrls(filepath)
	lineReader := bufio.NewReader(reader)
	for {
		site, err := lineReader.ReadString('\n')
		for _, prot := range prots {
			work <- formatUrl(prot, strings.TrimRight(site, "\n"))
		}
		if err != nil {
			break
		}
	}
}

func openUrls(filepath string) io.Reader {
	return strings.NewReader("google.com\nwww.google.com\ntweakers.net\nwww.tweakers.net\nsecurity.nl\nwww.security.nl\nwww.em-te.nl\ngmail.com\nwww.van-hoeckel.nl\nvan-hoeckel.nl")
}

func checkWorker(wg *sync.WaitGroup, work <-chan string, result chan<- *Site, check RedirectBehavior) {
	defer wg.Done()
	var c http.Client
	c.CheckRedirect = check
	for site := range work {
		r, err := testUrl(&c, site)
		if err == nil {
			result <- r
		} else {
			os.Stderr.WriteString(err.Error() + "\n")
		}
	}
}

func writeWorker(wg *sync.WaitGroup, result <-chan *Site) {
	defer wg.Done()
	for r := range result {
		os.Stdout.WriteString(r.String() + "\n")
	}
}

func testUrl(c *http.Client, site string) (*Site, error) {
	resp, err := c.Get(site)
	if err != nil {
		switch e := err.(type) {
		case *url.Error:
			if e.Err == errDone {
				// just an errDone, continue
				break
			}
			// an unexpected error
			return nil, err
		default:
			// an unexpected error
			return nil, err
		}
	}
	var result Site
	result.RequestUrl = *resp.Request.URL
	result.StatusCode = resp.StatusCode
	if loc, err := resp.Location(); err == nil {
		result.ResponseUrl = loc
	}
	return &result, nil
}

func formatUrl(protocol string, site string) string {
	return protocol + "://" + site + "/"
}
