package main

import (
	"bufio"
	"crypto/sha1"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

func main() {

	var keepAlives bool
	flag.BoolVar(&keepAlives, "keep-alive", false, "use HTTP keep-alives")

	var saveResponses bool
	flag.BoolVar(&saveResponses, "save", false, "save responses")

	var delayMs int
	flag.IntVar(&delayMs, "delay", 100, "delay between issuing requests (ms)")

	var outputDir string
	flag.StringVar(&outputDir, "output", "out", "output directory")

	var headers headerArgs
	flag.Var(&headers, "header", "add a header to the request")
	flag.Var(&headers, "H", "add a header to the request")

	flag.Parse()

	delay := time.Duration(delayMs * 1000000)
	client := newClient(keepAlives)
	prefix := outputDir

	var wg sync.WaitGroup

	sc := bufio.NewScanner(os.Stdin)

	for sc.Scan() {
		rawURL := sc.Text()
		wg.Add(1)

		time.Sleep(delay)

		go func() {
			defer wg.Done()

			// create the request
			req, err := http.NewRequest("GET", rawURL, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create request: %s\n", err)
				return
			}

			// add headers to the request
			for _, h := range headers {
				parts := strings.SplitN(h, ":", 2)

				if len(parts) != 2 {
					continue
				}
				req.Header.Set(parts[0], parts[1])
			}

			// send the request
			resp, err := client.Do(req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "request failed: %s\n", err)
				return
			}
			defer resp.Body.Close()

			if !saveResponses {
				_, _ = io.Copy(ioutil.Discard, resp.Body)
				fmt.Printf("%s %d\n", rawURL, resp.StatusCode)
				return
			}

			// output files are prefix/domain/hash.(body|headers)
			hash := sha1.Sum([]byte(rawURL))
			p := path.Join(prefix, req.URL.Hostname(), fmt.Sprintf("%x.body", hash))
			err = os.MkdirAll(path.Dir(p), 0750)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create dir: %s\n", err)
				return
			}

			// create the body file
			f, err := os.Create(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create file: %s\n", err)
				return
			}
			defer f.Close()

			_, err = io.Copy(f, resp.Body)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to write file contents: %s\n", err)
				return
			}

			// create the headers file
			headersPath := path.Join(prefix, req.URL.Hostname(), fmt.Sprintf("%x.headers", hash))
			headersFile, err := os.Create(headersPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create file: %s\n", err)
				return
			}
			defer headersFile.Close()

			var buf strings.Builder
			buf.WriteString(fmt.Sprintf("%s\n\n", rawURL))

			// add the request headers
			for _, h := range headers {
				buf.WriteString(fmt.Sprintf("> %s\n", h))
			}

			buf.WriteRune('\n')

			// add the response headers
			for k, vs := range resp.Header {
				for _, v := range vs {
					buf.WriteString(fmt.Sprintf("< %s: %s\n", k, v))
				}
			}

			_, err = io.Copy(headersFile, strings.NewReader(buf.String()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to write file contents: %s\n", err)
				return
			}

			// output the body filename for each URL
			fmt.Printf("%s: %s %d\n", p, rawURL, resp.StatusCode)
		}()
	}

	wg.Wait()

}

func newClient(keepAlives bool) *http.Client {

	tr := &http.Transport{
		MaxIdleConns:      30,
		IdleConnTimeout:   time.Second,
		DisableKeepAlives: !keepAlives,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   time.Second * 10,
			KeepAlive: time.Second,
		}).DialContext,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &http.Client{
		Transport:     tr,
		CheckRedirect: re,
		Timeout:       time.Second * 10,
	}

}

type headerArgs []string

func (h *headerArgs) Set(val string) error {
	*h = append(*h, val)
	return nil
}

func (h headerArgs) String() string {
	return "string"
}
