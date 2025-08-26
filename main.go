package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

type ProxyResponse struct {
	Status int    `json:"status"`
	Info   string `json:"info"`
	Data   []struct {
		IP   string `json:"ip"`
		Port string `json:"port"`
		Prov string `json:"prov"`
		City string `json:"city"`
	} `json:"data"`
}

type Board struct {
	UrlString   string
	TotalPage   int
	ArchiveTime int
}

const domainName = "ac1ss.ascwefkjw.com"
const baseUrlString = "https://ac1ss.ascwefkjw.com/"

var mobileTXTUrlString = "forum.php?mod=forumdisplay&fid=103"

var totalPages = 0

const getProxyStr = "http://api2.xkdaili.com/tools/XApi.ashx?apikey=XK18C8EF13FE102BF294&qty=36&format=json&split=0&sign=9bcab6037ea8275aebe49d5c04989c4a"

var ips = make([]string, 36)

func main() {
	var boards [6]Board

	boards[0] = Board{UrlString: "forum.php?mod=forumdisplay&fid=102", TotalPage: 355, ArchiveTime: 2015}
	boards[1] = Board{UrlString: "forum.php?mod=forumdisplay&fid=101", TotalPage: 338, ArchiveTime: 2018}
	boards[2] = Board{UrlString: "forum.php?mod=forumdisplay&fid=100", TotalPage: 730, ArchiveTime: 2020}
	boards[3] = Board{UrlString: "forum.php?mod=forumdisplay&fid=99", TotalPage: 381, ArchiveTime: 2021}
	boards[4] = Board{UrlString: "forum.php?mod=forumdisplay&fid=104", TotalPage: 1889, ArchiveTime: 2022}
	boards[5] = Board{UrlString: "forum.php?mod=forumdisplay&fid=103", TotalPage: 923, ArchiveTime: 2023}

	scrapeBoard(boards[0])
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func scrapeBoard(board Board) {
	mobileTXTUrlString = board.UrlString
	totalPages = board.TotalPage
	var wg sync.WaitGroup

	resp, err := http.Get(getProxyStr)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var response ProxyResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		panic(err)
	}
	for i, item := range response.Data {
		ips[i] = "http://" + item.IP + ":" + item.Port
		fmt.Println(ips[i])
	}

	tidChannel := make(chan int)
	for i := 1; i <= totalPages; i++ {
		wg.Add(1)
		go scrapeBookListPage(i, tidChannel, &wg)
	}

	go func() {
		wg.Wait()
		close(tidChannel)
	}()

	var tids []int
	for v := range tidChannel {
		tids = append(tids, v)
	}
	_ = os.WriteFile("data.json", must(json.Marshal(tids)), 0644)
	fmt.Printf("✡️ Finished! total:%d\n", len(tids))
}

func fetchUrl(targetURL string, index int) ([]byte, error) {
	proxyStr := ips[index/10]
	fmt.Println("using:" + proxyStr)
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyURL(proxyURL),
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Cookie", "PHPSESSID=fehap6pvv43kuu7r4foskqi2dp; yj0M_5a0c_saltkey=FEBk1Iww; yj0M_5a0c_lastvisit=1756033217; yj0M_5a0c_lastact=1756037036%09index.php%09; yj0M_5a0c_st_t=0%7C1756036817%7Cabc85afee8e05d695b2929267559e611; yj0M_5a0c_sendmail=1; yj0M_5a0c_ulastactivity=1756037036%7C0; yj0M_5a0c_auth=fd77dVk912FxISusbqKRBKVlMjs03ZZkRYjdjkPWvFCSiZRwhxuZbdKDf2tWbf%2FUKYjlllF4mmvMaW2ssR09ck9desRe; yj0M_5a0c_lastcheckfeed=2527028%7C1756037036; yj0M_5a0c_checkfollow=1; yj0M_5a0c_lip=47.79.94.249%2C1756037036; yj0M_5a0c_sid=0")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code %d from %s", resp.StatusCode, targetURL)
	}

	contentType := resp.Header.Get("Content-Type")
	reader, err := charset.NewReader(resp.Body, contentType)
	if err != nil {
		return nil, fmt.Errorf("charset reader: %w", err)
	}

	buf, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return buf, nil
}

func scrapeBookListPage(page int, ch chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	BookListPageUrlString := baseUrlString + mobileTXTUrlString + "&page=" + strconv.Itoa(page)
	fmt.Printf("🌟 Scraping Page %d ...\n", page)
	body, err := fetchUrl(BookListPageUrlString, page)
	if err != nil {
		panic(err)
	}

	reader := bytes.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		panic(err)
	}

	doc.Find("#threadlisttableid > tbody").Each(func(i int, s *goquery.Selection) {
		id, exists := s.Attr("id")
		if exists {
			if strings.Contains(id, "normalthread") {
				href, exists := s.Find("tr:nth-child(1) > th:nth-child(2) > a:nth-child(3)").Attr("href")
				if exists {
					u, err := url.Parse(href)
					if err != nil {
						panic(err)
					}

					query, err := url.ParseQuery(u.RawQuery)
					if err != nil {
						panic(err)
					}

					tid := query.Get("tid")

					if tid != "adver" {
						tidInt, err := strconv.Atoi(tid)
						if err != nil {
							panic(err)
						}
						ch <- tidInt
					}
				}
			}
		}
	})
}
