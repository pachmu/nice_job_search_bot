package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type page struct {
	Hits []hit `json:"hits"`
}

type hit struct {
	Url string `json:"url"`
}

func Start(ctx context.Context) (<-chan map[string]struct{}, <-chan error) {
	outChan := make(chan map[string]struct{})
	errChan := make(chan error)
	go func() {
		err := doJob(ctx, outChan)
		select {
		case errChan <- err:
		case <-ctx.Done():
		}
	}()
	return outChan, errChan
}

func doJob(ctx context.Context, out chan<- map[string]struct{}) error {
	for {
		timer := time.NewTimer(time.Minute)
		select {
		case <-timer.C:
		case <-ctx.Done():
			logrus.Info("Job search has been stopped.")
			timer.Stop()
			return nil
		}
		date := time.Now()
		if date.Hour() != 19 || date.Minute() != 0 {
			continue
		}
		errGr, errGrCtx := errgroup.WithContext(ctx)
		links := make(chan string)
		output := make(chan string)
		result := make(map[string]struct{})
		tr := &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 5 * time.Second,
		}
		client := &http.Client{
			Transport: tr,
			Timeout:   5 * time.Second,
		}
		for i := 0; i < 10; i++ {
			errGr.Go(func() error {
				for {
					select {
					case link := <-links:
						if link == "" {
							return nil
						}
						purl, err := url.Parse(link)
						if err != nil {
							return err
						}
						link = strings.ReplaceAll(purl.Host, "blog.", "")
						careersLink := fmt.Sprintf("%s://%s/careers", purl.Scheme, link)
						resp, err := client.Get(careersLink)
						if err != nil {
							logrus.Error(err)
							continue
						}

						if resp.StatusCode == 404 {
							resp.Body.Close()
							prefix := "careers."
							if strings.Contains(link, "www.") {
								link = strings.Replace(link, "www.", "", 1)
								prefix = "www." + prefix
							}
							careersLink = fmt.Sprintf("%s://%s%s", purl.Scheme, prefix, link)
							resp, err = client.Get(careersLink)
							if err != nil {
								logrus.Error(err)
								continue
							}
						}
						if resp.StatusCode != http.StatusOK {
							resp.Body.Close()
							continue
						}
						body, err := io.ReadAll(resp.Body)
						resp.Body.Close()
						if err != nil {
							return err
						}
						stringBody := string(body)
						if strings.Contains(stringBody, "fully remote") ||
							strings.Contains(stringBody, "Fully remote") ||
							strings.Contains(stringBody, "remote-first") ||
							strings.Contains(stringBody, "Remote-first") ||
							strings.Contains(stringBody, "remote first") ||
							strings.Contains(stringBody, "Remote first") ||
							strings.Contains(stringBody, "Worldwide") ||
							strings.Contains(stringBody, "worldwide") {
							select {
							case output <- careersLink:
							case <-errGrCtx.Done():
								return nil
							}
							continue
						}
					case <-ctx.Done():
						return nil
					}

				}
			})
		}
		errGr.Go(func() error {
			defer close(links)
			for i := 0; i < 11; i++ {
				resp, err := client.Post(
					fmt.Sprintf("https://uj5wyc0l7x-dsn.algolia.net/1/indexes/Item_dev_sort_date/query?x-algolia-agent=Algolia%20for%20JavaScript%20(4.13.1)%3B%20Browser%20(lite)&x-algolia-api-key=28f0e1ec37a5e792e6845e67da5f20dd&x-algolia-application-id=UJ5WYC0L7X"),
					"application/x-www-form-urlencoded",
					strings.NewReader(`{"query":"blog","analyticsTags":["web"],"page":`+strconv.Itoa(i)+`,"hitsPerPage":100,"minWordSizefor1Typo":4,"minWordSizefor2Typos":8,"advancedSyntax":true,"ignorePlurals":false,"clickAnalytics":true,"minProximity":7,"numericFilters":["created_at_i>1698424156.985"],"tagFilters":[["story"],[]],"typoTolerance":"min","queryType":"prefixNone","restrictSearchableAttributes":["title","comment_text","url","story_text","author"],"getRankingInfo":true}`),
				)
				if err != nil {
					return err
				}
				var pg page
				err = json.NewDecoder(resp.Body).Decode(&pg)
				resp.Body.Close()
				if err != nil {
					return err
				}
				for _, ht := range pg.Hits {
					if strings.Contains(ht.Url, "blog") {
						select {
						case links <- ht.Url:
						case <-errGrCtx.Done():
							return nil
						}
					}
				}
				time.Sleep(50 * time.Millisecond)
			}
			return nil
		})
		go func() {
			for {
				select {
				case o := <-output:
					result[o] = struct{}{}
				case <-errGrCtx.Done():
					return
				}
			}
		}()
		err := errGr.Wait()
		if err != nil {
			return err
		}
		select {
		case out <- result:
		case <-ctx.Done():
			return nil
		}
		logrus.Info("Job is done.")
	}
}
