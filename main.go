package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/saintfish/chardet"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	_ "modernc.org/sqlite"
)

var db *sql.DB
var dbMutex sync.Mutex

const baseUrlString = "https://ac1ss.ascwefkjw.com/"
const mobileTXTUrlString = "forum.php?mod=forumdisplay&fid=103"

const totalPages = 923

func main() {
	db, _ = sql.Open("sqlite", "soushu-mobileTXT-2023.db")
	defer db.Close()

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS novel (
	    tid INTEGER PRIMARY KEY,
	    title TEXT,
	    content TEXT
	);`
	_, err := db.Exec(createTableSQL)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup

	for i := 24; i <= totalPages; i++ {
		tidChannel := make(chan int)
		wg.Add(1)
		go scrapeBookListPage(i, tidChannel, &wg)

		go func() {
			wg.Wait()
			close(tidChannel)
		}()

		tids := make([]int, 0)
		for r := range tidChannel {
			tids = append(tids, r)
		}
		fmt.Printf("✡️ Finished! total:%d\n", len(tids))

		for i := range tids {
			query := `SELECT EXISTS(SELECT 1 FROM novel WHERE tid=? LIMIT 1)`
			row := db.QueryRow(query, tids[i])
			var exists bool
			if err := row.Scan(&exists); err != nil {
				panic(err)
			}
			if exists {
				fmt.Printf("📢 tid:%d Existed!\n", tids[i])
				continue
			}
			wg.Add(1)
			go scrapeBookPage(tids[i], db, &wg)
		}

		wg.Wait()
	}
}

func fetchUrl(targetURL string) ([]byte, error) {
	proxyStr := "http://127.0.0.1:7890"
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Cookie", "PHPSESSID=fehap6pvv43kuu7r4foskqi2dp; yj0M_5a0c_ulastactivity=1755685095%7C0; yj0M_5a0c_saltkey=wTb5t5gB; yj0M_5a0c_lastvisit=1755680847; yj0M_5a0c_lastact=1755685522%09forum.php%09viewthread; yj0M_5a0c__refer=%252Fhome.php%253Fmod%253Dspacecp%2526ac%253Dprofile%2526op%253Dpassword; yj0M_5a0c_auth=cf10GXGgtSukhwCS4cImZ9JYBSVvTghCdTsq1ht4qyebLeDNdTprQaXQm7vXC%2FDyke9L51ISDV24Z%2FENcwBPiu43hek; yj0M_5a0c_lastcheckfeed=588604%7C1755684455; yj0M_5a0c_lip=47.79.94.249%2C1755684455; yj0M_5a0c_sid=0; yj0M_5a0c_nofavfid=1; yj0M_5a0c_st_t=588604%7C1755685250%7C1cb744b1014d3a5af96469584e0de3cb; yj0M_5a0c_forum_lastvisit=D_102_1755684843D_72_1755684870D_103_1755685011D_40_1755685250; yj0M_5a0c_smile=1D1; yj0M_5a0c_st_p=588604%7C1755685522%7C591978a0f584979afc565ead1ec9e755; yj0M_5a0c_viewid=tid_1324402")
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
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("parse content-type: %w", err)
	}

	charsetStr := params["charset"]

	if charsetStr != "" {
		reader, err := charset.NewReader(resp.Body, contentType)
		if err != nil {
			return nil, fmt.Errorf("charset reader: %w", err)
		}

		buf, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		if strings.HasPrefix(string(buf), "<script") {
			fmt.Printf("🔑 JS Challenge: %s\n", string(buf)[:20])

			ctx, cancel := chromedp.NewContext(context.Background())
			defer cancel()

			var url string
			chromedp.Run(ctx,
				chromedp.Navigate(targetURL),
				chromedp.Evaluate(`document.location.href`, &url),
			)

			return fetchUrl(url)
		}
		if strings.Contains(string(buf), "您浏览的太快了，歇一会儿吧！") {
			fmt.Println("⌛ Take A Break")
			time.Sleep(1 * time.Minute)
			return fetchUrl(targetURL)
		}
		return buf, nil
	} else {
		utf8Data, err := ConvertToUTF8(resp.Body)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(string(utf8Data), "<script") {
			fmt.Printf("🔑 JS Challenge: %s\n", string(utf8Data)[:20])

			ctx, cancel := chromedp.NewContext(context.Background())
			defer cancel()

			var url string
			chromedp.Run(ctx,
				chromedp.Navigate(targetURL),
				chromedp.Evaluate(`document.location.href`, &url),
			)

			return fetchUrl(url)
		}
		if strings.Contains(string(utf8Data), "您浏览的太快了，歇一会儿吧！") {
			fmt.Println("⌛ Take A Break")
			time.Sleep(1 * time.Minute)
			return fetchUrl(targetURL)
		}
		return utf8Data, nil
	}
}

func scrapeBookPage(tid int, db *sql.DB, wg *sync.WaitGroup) {
	defer wg.Done()

	BookPageUrlString := baseUrlString + "forum.php?mod=viewthread&tid=" + strconv.Itoa(tid)
	fmt.Printf("📖 Scraping Book %d ...\n", tid)

	var body []byte
	var err error
	for {
		body, err = fetchUrl(BookPageUrlString)
		if err == nil {
			break
		}
		fmt.Printf("⌛ retry: %s, %v\n", BookPageUrlString, err)
		time.Sleep(2 * time.Second)
	}

	reader := bytes.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		panic(err)
	}

	title := doc.Find("#thread_subject").First().Text()

	content := ""
	doc.Find("#postlist > div:nth-child(3) a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "forum.php?mod=attachment&aid=") {
				if !strings.HasSuffix(s.Text(), ".txt") {
					return
				}
				fileUrlString := baseUrlString + href
				// fmt.Printf("📁 Find File: %s, Link: %s\n", s.Text(), fileUrlString)
				var txt []byte
				for {
					txt, err = fetchUrl(fileUrlString)
					if err == nil {
						break
					}
					fmt.Printf("⌛ retry: %s, %v\n", fileUrlString, err)
					time.Sleep(2 * time.Second)
				}
				content = content + string(txt) + "\n"
			}
		}
	})

	fmt.Printf("⚜️ Book Compeleted: %s, Content Length: %d\n", title, len(content))

	dbMutex.Lock()
	insertSQL := `INSERT INTO novel (tid, title, content) VALUES (?, ?, ?)`
	_, err = db.Exec(insertSQL, tid, title, content)
	if err != nil {
		panic(err)
	}
	dbMutex.Unlock()
}

func scrapeBookListPage(page int, ch chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	BookListPageUrlString := baseUrlString + mobileTXTUrlString + "&page=" + strconv.Itoa(page)
	fmt.Printf("🌟 Scraping Page %d ...\n", page)
	var body []byte
	var err error
	for {
		body, err = fetchUrl(BookListPageUrlString)
		if err == nil {
			break
		}
		fmt.Printf("⌛ retry: %s, %v\n", BookListPageUrlString, err)
		time.Sleep(2 * time.Second)
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

func ConvertToUTF8(body io.ReadCloser) ([]byte, error) {
	defer body.Close()

	// 读取全部内容
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	// 检测编码
	detector := chardet.NewTextDetector()
	result, err := detector.DetectBest(data)
	if err != nil {
		return nil, err
	}

	var enc encoding.Encoding

	if result.Charset == "UTF-8" {
		return data, nil
	} else {
		enc = simplifiedchinese.GB18030
		// 转换为 UTF-8
		reader := transform.NewReader(bytes.NewReader(data), enc.NewDecoder())
		utf8Data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}

		return utf8Data, nil
	}
}
