package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	// NumCheckers contains the number of workers that are used to check URLs.
	NumCheckers = 5
)

var (
	errDone   = errors.New("not following any more redirects")
	protocols = []string{
		"http",
		"https",
	}
)

// Redirector is the function signature for a redirect checker for http requests.
type Redirector func(req *http.Request, via []*http.Request) error

var (
	// FollowAllRedirects follows all redirects, unconditionally.
	FollowAllRedirects = func(req *http.Request, via []*http.Request) error {
		return nil
	}
	// StopOnFirstRedirect stops at the first redirect it encounters..
	StopOnFirstRedirect = func(req *http.Request, via []*http.Request) error {
		return errDone
	}
	// StopOnRedirectToDifferentDomain stops as soon as you are redirected to a different domain.
	StopOnRedirectToDifferentDomain = func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != via[len(via)-1].URL.Host {
			return errDone
		}
		return nil
	}
	// StopOnCyclicRedirect stops only if redirects get into an infinite loop.
	StopOnCyclicRedirect = func(req *http.Request, via []*http.Request) error {
		for _, prev := range via {
			if *prev.URL == *req.URL {
				return errDone
			}
		}
		return nil
	}
)

// Site contains the request URL, HTTP status code and response URL.
type Site struct {
	RequestURL  url.URL
	StatusCode  int
	ResponseURL *url.URL
}

func (s *Site) String() string {
	var line = fmt.Sprintf("%s,%d,", s.RequestURL.String(), s.StatusCode)
	if s.ResponseURL != nil {
		line += s.ResponseURL.String()
	}
	return line
}

func main() {
	flag.Parse()
	// start result writer
	var writerGroup sync.WaitGroup
	var result = make(chan *Site)
	writerGroup.Add(1)
	go writeWorker(&writerGroup, result)
	// start url reader
	var work = make(chan string)
	go readWorker(work)
	// start checkers
	var checkRedirect = StopOnFirstRedirect
	var workerGroup sync.WaitGroup
	for i := 0; i < NumCheckers; i++ {
		workerGroup.Add(1)
		go checkWorker(&workerGroup, work, result, checkRedirect)
	}
	// wait for all checkers to finish
	workerGroup.Wait()
	close(result)
	// wait for writer to finish
	writerGroup.Wait()
}

func readWorker(work chan<- string) {
	defer close(work)
	var source io.Reader
	if flag.NArg() < 1 {
		source = os.Stdin
	} else {
		var err error
		source, err = os.Open(flag.Arg(0))
		if err != nil {
			os.Stderr.WriteString("error getting URLs: " + err.Error() + "\n")
			return
		}
	}
	lineReader := bufio.NewReader(source)
	for {
		site, err := lineReader.ReadString('\n')
		if len(site) > 0 {
			for _, prot := range protocols {
				work <- formatURL(prot, strings.Trim(site, " \t\r\n"))
			}
		}
		if err != nil {
			break
		}
	}
}

func checkWorker(wg *sync.WaitGroup, work <-chan string, result chan<- *Site, check Redirector) {
	defer wg.Done()
	var c http.Client
	c.CheckRedirect = check
	for site := range work {
		r, err := testURL(&c, site)
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

func testURL(c *http.Client, site string) (*Site, error) {
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
	result.RequestURL = *resp.Request.URL
	result.StatusCode = resp.StatusCode
	if loc, err := resp.Location(); err == nil {
		result.ResponseURL = loc
	}
	return &result, nil
}

func formatURL(protocol string, site string) string {
	return protocol + "://" + site + "/"
}
