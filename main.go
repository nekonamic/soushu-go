package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
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
const mobileTXTUrlString = "forum.php?mod=forumdisplay&fid=40"

const maxGoroutines = 1000

var sem = make(chan struct{}, maxGoroutines)

func main() {
	db, _ = sql.Open("sqlite", "soushu-mobileTXT.db")
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

	totalPages := 1
	tidChannel := make(chan int)
	var wg sync.WaitGroup

	for i := 1; i <= totalPages; i++ {
		wg.Add(1)
		go scrapeBookListPage(i, tidChannel, &wg)
	}

	go func() {
		wg.Wait()
		close(tidChannel)
	}()

	tids := make([]int, 0)
	for r := range tidChannel {
		tids = append(tids, r)
	}
	fmt.Printf("✡️ Finished! total:%d\n", len(tids))

	for i := 0; i < len(tids); i++ {
		wg.Add(1)
		sem <- struct{}{}
		go scrapeBookPage(tids[i], db, &wg)
	}

	wg.Wait()
}

func fetchWithProxy(targetURL string, proxyStr string, timeout time.Duration) ([]byte, error) {
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %s: %w", proxyStr, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: timeout,
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Cookie", "PHPSESSID=fehap6pvv43kuu7r4foskqi2dp; yj0M_5a0c_ulastactivity=1755685095%7C0; yj0M_5a0c_saltkey=wTb5t5gB; yj0M_5a0c_lastvisit=1755680847; yj0M_5a0c_lastact=1755685522%09forum.php%09viewthread; yj0M_5a0c__refer=%252Fhome.php%253Fmod%253Dspacecp%2526ac%253Dprofile%2526op%253Dpassword; yj0M_5a0c_auth=cf10GXGgtSukhwCS4cImZ9JYBSVvTghCdTsq1ht4qyebLeDNdTprQaXQm7vXC%2FDyke9L51ISDV24Z%2FENcwBPiu43hek; yj0M_5a0c_lastcheckfeed=588604%7C1755684455; yj0M_5a0c_lip=47.79.94.249%2C1755684455; yj0M_5a0c_sid=0; yj0M_5a0c_nofavfid=1; yj0M_5a0c_st_t=588604%7C1755685250%7C1cb744b1014d3a5af96469584e0de3cb; yj0M_5a0c_forum_lastvisit=D_102_1755684843D_72_1755684870D_103_1755685011D_40_1755685250; yj0M_5a0c_smile=1D1; yj0M_5a0c_st_p=588604%7C1755685522%7C591978a0f584979afc565ead1ec9e755; yj0M_5a0c_viewid=tid_1324402")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Go-http-client/1.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error accessing %s via proxy %s: %w", targetURL, proxyStr, err)
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
		defer resp.Body.Close()

		buf, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		return buf, nil
	} else {
		utf8Data, err := DetectAndConvertToUTF8(resp.Body)
		if err != nil {
			panic(err)
		}
		return utf8Data, nil
	}
}

func scrapeBookPage(tid int, db *sql.DB, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		<-sem
	}()

	countries := []string{"AL", "AR", "AM", "AU", "AT", "AZ", "BD", "BY", "BE", "BO", "BR", "BG", "KH", "CA", "CL", "CN", "CO", "CR", "HR", "CY", "CZ", "DK", "DO", "EC", "EG", "EE", "FI", "FR", "GE", "DE", "GB", "GR", "HK", "HU", "IS", "IN", "ID", "IE", "IM", "IL", "IT", "JM", "JP", "JO", "KZ", "KG", "LA", "LV", "LT", "LU", "MY", "MX", "MD", "MA", "NL", "NZ", "NO", "PK", "PA", "PE", "PH", "PL", "PT", "RO", "RU", "SA", "SG", "SK", "ZA", "KR", "ES", "LK", "SE", "CH", "TW", "TJ", "TH", "TR", "TM", "UA", "AE", "US", "UZ", "VN"}
	BookPageUrlString := baseUrlString + "forum.php?mod=viewthread&tid=" + strconv.Itoa(tid)
	fmt.Printf("📖 Scraping Book %d ...\n", tid)

	randomIndex := rand.Intn(9998) + 1
	randomCountry := countries[rand.Intn(len(countries))]

	proxyStr := fmt.Sprintf("http://oc-ab35117cd53146a5be0297f8b368339cb877481a6178015c64d8599f75127596-country-%s-session-%d:it5q47pegzmt@proxy.oculus-proxy.com:31114", randomCountry, randomIndex)
	body, err := fetchWithProxy(BookPageUrlString, proxyStr, 6000*time.Second)
	if err != nil {
		panic(err)
	}

	reader := bytes.NewReader(body)
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		panic(err)
	}

	title := doc.Find("#thread_subject").First().Text()

	content := ""
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			if strings.HasPrefix(href, "forum.php?mod=attachment&aid=") {
				fileUrlString := baseUrlString + href
				fmt.Printf("📁 Find File: %s, Link: %s\n", s.Text(), fileUrlString)
				txt, err := fetchWithProxy(fileUrlString, proxyStr, 6000*time.Second)
				if err != nil {
					panic(err)
				}
				content = content + string(txt) + "\n"
			}
		}
	})

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
	proxyStr := fmt.Sprintf("http://oc-ab35117cd53146a5be0297f8b368339cb877481a6178015c64d8599f75127596-country-US-session-%d:it5q47pegzmt@proxy.oculus-proxy.com:31114", page%9999)
	body, err := fetchWithProxy(BookListPageUrlString, proxyStr, 6000*time.Second)
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

// DetectAndConvertToUTF8 读取 resp.Body，自动检测编码并转换为 UTF-8
func DetectAndConvertToUTF8(body io.ReadCloser) ([]byte, error) {
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

	switch result.Charset {
	case "UTF-8":
		return data, nil // 已经是 UTF-8，无需转换
	case "GB-18030", "GB-2312", "GBK":
		enc = simplifiedchinese.GB18030
	default:
		// 可以扩展其他编码，或者直接返回原始字节
		return data, fmt.Errorf("unsupported charset: %s", result.Charset)
	}

	// 转换为 UTF-8
	reader := transform.NewReader(bytes.NewReader(data), enc.NewDecoder())
	utf8Data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return utf8Data, nil
}
